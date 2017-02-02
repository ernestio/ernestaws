/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package route53

import (
	"encoding/json"
	"errors"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/ernestio/ernestaws/credentials"
)

// Collection ....
type Collection struct {
	UUID               string            `json:"_uuid"`
	BatchID            string            `json:"_batch_id"`
	ProviderType       string            `json:"_type"`
	Service            string            `json:"service"`
	AWSAccessKeyID     string            `json:"aws_access_key_id"`
	AWSSecretAccessKey string            `json:"aws_secret_access_key"`
	DatacenterRegion   string            `json:"datacenter_region"`
	Tags               map[string]string `json:"tags"`
	Results            []interface{}     `json:"components"`
	ErrorMessage       string            `json:"error,omitempty"`
	Subject            string            `json:"-"`
	Body               []byte            `json:"-"`
	CryptoKey          string            `json:"-"`
}

// GetBody : Gets the body for this event
func (col *Collection) GetBody() []byte {
	var err error
	if col.Body, err = json.Marshal(col); err != nil {
		log.Println(err.Error())
	}
	return col.Body
}

// GetSubject : Gets the subject for this event
func (col *Collection) GetSubject() string {
	return col.Subject
}

// Process : starts processing the current message
func (col *Collection) Process() (err error) {
	if err := json.Unmarshal(col.Body, &col); err != nil {
		col.Error(err)
		return err
	}

	if err := col.Validate(); err != nil {
		col.Error(err)
		return err
	}

	return nil
}

// Error : Will respond the current event with an error
func (col *Collection) Error(err error) {
	log.Printf("Error: %s", err.Error())
	col.ErrorMessage = err.Error()

	col.Body, err = json.Marshal(col)
}

// Validate checks if all criteria are met
func (col *Collection) Validate() error {
	if col.AWSAccessKeyID == "" || col.AWSSecretAccessKey == "" {
		return ErrDatacenterCredentialsInvalid
	}

	return nil
}

// Get : Gets a object on aws
func (col *Collection) Get() error {
	return errors.New(col.Subject + " not supported")
}

// Create : Creates an object on aws
func (col *Collection) Create() error {
	return errors.New(col.Subject + " not supported")
}

// Update : Updates an object on aws
func (col *Collection) Update() error {
	return errors.New(col.Subject + " not supported")
}

// Delete : Delete an object on aws
func (col *Collection) Delete() error {
	return errors.New(col.Subject + " not supported")
}

func (col *Collection) getRoute53Client() *route53.Route53 {
	creds, _ := credentials.NewStaticCredentials(col.AWSAccessKeyID, col.AWSSecretAccessKey, col.CryptoKey)
	return route53.New(session.New(), &aws.Config{
		Region:      aws.String(col.DatacenterRegion),
		Credentials: creds,
	})
}

// Find : Find route53 zones on aws
func (col *Collection) Find() error {
	svc := col.getRoute53Client()

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

		event := toEvent(z, records, tags)

		if tagsMatch(col.Tags, event.Tags) {
			col.Results = append(col.Results, event)
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
	if err != nil {
		return []*route53.ResourceRecordSet{}, err
	}

	return resp.ResourceRecordSets, nil
}

func getZoneTagDescriptions(svc *route53.Route53, id *string) ([]*route53.Tag, error) {
	req := &route53.ListTagsForResourceInput{
		ResourceId:   id,
		ResourceType: aws.String("hostedzone"),
	}

	resp, err := svc.ListTagsForResource(req)
	if err != nil {
		return []*route53.Tag{}, err
	}

	return resp.ResourceTagSet.Tags, nil
}

func mapRoute53Tags(input []*route53.Tag) map[string]string {
	t := make(map[string]string)

	for _, tag := range input {
		t[*tag.Key] = *tag.Value
	}

	return t
}

func mapRecordValues(rv []*route53.ResourceRecord) []*string {
	var values []*string

	for _, v := range rv {
		values = append(values, v.Value)
	}

	return values
}

func mapRoute53Records(name *string, records []*route53.ResourceRecordSet) []Record {
	var zr []Record

	for _, r := range records {
		if isDefaultRule(*name, r) != true {
			zr = append(zr, Record{
				Entry:  r.Name,
				Type:   r.Type,
				TTL:    r.TTL,
				Values: mapRecordValues(r.ResourceRecords),
			})
		}
	}

	return zr
}

// ToEvent converts an route53 instance object to an ernest event
func toEvent(z *route53.HostedZone, records []*route53.ResourceRecordSet, tags []*route53.Tag) *Event {
	e := &Event{
		HostedZoneID: z.Id,
		Name:         z.Name,
		Private:      z.Config.PrivateZone,
		Records:      mapRoute53Records(z.Name, records),
		Tags:         mapRoute53Tags(tags),
	}
	return e
}
