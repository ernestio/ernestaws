/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package elb

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/ernestio/ernestaws"
	"github.com/ernestio/ernestaws/credentials"
)

func getELBClient(q *ernestaws.Query) *elb.ELB {
	creds, _ := credentials.NewStaticCredentials(q.AWSAccessKeyID, q.AWSSecretAccessKey, q.CryptoKey)
	return elb.New(session.New(), &aws.Config{
		Region:      aws.String(q.DatacenterRegion),
		Credentials: creds,
	})
}

// FindELBs : Find elbs on aws
func FindELBs(q *ernestaws.Query) error {
	svc := getELBClient(q)

	resp, err := svc.DescribeLoadBalancers(nil)
	if err != nil {
		return err
	}

	for _, e := range resp.LoadBalancerDescriptions {
		req := &elb.DescribeTagsInput{
			LoadBalancerNames: []*string{e.LoadBalancerName},
		}

		resp, err := svc.DescribeTags(req)
		if err != nil {
			return err
		}

		event := ToEvent(e, resp.TagDescriptions[0].Tags)

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

func mapELBTags(input []*elb.Tag) map[string]string {
	t := make(map[string]string)

	for _, tag := range input {
		t[*tag.Key] = *tag.Value
	}

	return t
}

func mapELBListeners(input []*elb.ListenerDescription) []Listener {
	var listeners []Listener

	for _, ld := range input {
		listeners = append(listeners, Listener{
			FromPort:  *ld.Listener.LoadBalancerPort,
			ToPort:    *ld.Listener.InstancePort,
			Protocol:  *ld.Listener.Protocol,
			SSLCertID: *ld.Listener.SSLCertificateId,
		})
	}

	return listeners
}

func mapELBSecurityGroups(input []*string) []string {
	var sgs []string

	for _, sg := range input {
		sgs = append(sgs, *sg)
	}

	return sgs
}

func mapELBInstances(input []*elb.Instance) []string {
	var instances []string

	for _, i := range input {
		instances = append(instances, *i.InstanceId)
	}

	return instances
}

func mapELBSubnets(input []*string) []string {
	var subnets []string

	for _, s := range input {
		subnets = append(subnets, *s)
	}

	return subnets
}

// ToEvent converts an ec2 subnet object to an ernest event
func ToEvent(e *elb.LoadBalancerDescription, tags []*elb.Tag) *Event {
	return &Event{
		VPCID:               *e.VPCId,
		ELBName:             *e.LoadBalancerName,
		ELBDNSName:          *e.DNSName,
		ELBListeners:        mapELBListeners(e.ListenerDescriptions),
		InstanceAWSIDs:      mapELBInstances(e.Instances),
		NetworkAWSIDs:       mapELBSubnets(e.Subnets),
		SecurityGroupAWSIDs: mapELBSecurityGroups(e.SecurityGroups),
		Tags:                mapELBTags(tags),
		//ELBIsPrivate: *e.,
	}
}
