/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package route53

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/ernestio/ernestaws"
	"github.com/ernestio/ernestaws/credentials"
)

func getRoute53Client(q *ernestaws.Query) *route53.Route53 {
	creds, _ := credentials.NewStaticCredentials(q.AWSAccessKeyID, q.AWSSecretAccessKey, q.CryptoKey)
	return route53.New(session.New(), &aws.Config{
		Region:      aws.String(q.DatacenterRegion),
		Credentials: creds,
	})
}

// FindRoute53Zones : Find route53 zones on aws
func FindRoute53Zones(q *ernestaws.Query) error {
	svc := getRoute53Client(q)

	resp, err := svc.ListHostedZones(nil)
	if err != nil {
		return err
	}

	for _, z := range resp.HostedZones {
		tags, err := getZoneTagDescriptions(svc, z.Id)
		if err != nil {
			return err
		}

		records, err := getZoneRecords(svc, z.Id)
		if err != nil {
			return err
		}

		event := ToEvent(z, records, tags)

		if tagsMatch(q.Tags, event.Tags) {
			q.Results = append(q.Results, event)
		}
	}

	return nil
}

func tagsMatch(qt, rt map[string]string) bool {
	for k, v := range qt {
		if rt[k] != v {
			return false
		}
	}

	return true
}

func getZoneRecords(svc *route53.Route53, id *string) ([]*route53.ResourceRecordSet, error) {
	zreq := &route53.ListResourceRecordSetsInput{
		HostedZoneId: id,
	}

	resp, err := svc.ListResourceRecordSets(zreq)

	return resp.ResourceRecordSets, err
}

func getZoneTagDescriptions(svc *route53.Route53, id *string) ([]*route53.Tag, error) {
	treq := &route53.ListTagsForResourceInput{
		ResourceId: id,
	}

	resp, err := svc.ListTagsForResource(treq)

	return resp.ResourceTagSet.Tags, err
}

func mapRoute53Tags(input []*route53.Tag) map[string]string {
	t := make(map[string]string)

	for _, tag := range input {
		t[*tag.Key] = *tag.Value
	}

	return t
}

func mapRecordValues(rv []*route53.ResourceRecord) []string {
	var values []string

	for _, v := range rv {
		values = append(values, *v.Value)
	}

	return values
}

func mapRoute53Records(records []*route53.ResourceRecordSet) []Record {
	var zr []Record

	for _, r := range records {
		zr = append(zr, Record{
			Entry:  *r.Name,
			Type:   *r.Type,
			TTL:    *r.TTL,
			Values: mapRecordValues(r.ResourceRecords),
		})
	}

	return zr
}

// ToEvent converts an route53 instance object to an ernest event
func ToEvent(z *route53.HostedZone, records []*route53.ResourceRecordSet, tags []*route53.Tag) *Event {
	e := &Event{
		HostedZoneID: *z.Id,
		Name:         *z.Name,
		Private:      *z.Config.PrivateZone,
		Records:      mapRoute53Records(records),
		Tags:         mapRoute53Tags(tags),
	}
	return e
}
