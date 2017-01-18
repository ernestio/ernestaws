/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package elb

import (
	"encoding/json"
	"errors"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
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

// Find : Find elbs on aws
func (col *Collection) Find() error {
	svc := col.getELBClient()

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

		event := toEvent(e, resp.TagDescriptions[0].Tags)

		if tagsMatch(col.Tags, event.Tags) {
			col.Results = append(col.Results, event)
		}
	}

	return nil
}

func (col *Collection) getELBClient() *elb.ELB {
	creds, _ := credentials.NewStaticCredentials(col.AWSAccessKeyID, col.AWSSecretAccessKey, col.CryptoKey)
	return elb.New(session.New(), &aws.Config{
		Region:      aws.String(col.DatacenterRegion),
		Credentials: creds,
	})
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
			FromPort:  ld.Listener.LoadBalancerPort,
			ToPort:    ld.Listener.InstancePort,
			Protocol:  ld.Listener.Protocol,
			SSLCertID: ld.Listener.SSLCertificateId,
		})
	}

	return listeners
}

func mapELBSecurityGroups(input []*string) []*string {
	var sgs []*string

	for _, sg := range input {
		sgs = append(sgs, sg)
	}

	return sgs
}

func mapELBInstances(input []*elb.Instance) []*string {
	var instances []*string

	for _, i := range input {
		instances = append(instances, i.InstanceId)
	}

	return instances
}

func mapELBSubnets(input []*string) []*string {
	var subnets []*string

	for _, s := range input {
		subnets = append(subnets, s)
	}

	return subnets
}

// toEvent converts an ec2 subnet object to an ernest event
func toEvent(e *elb.LoadBalancerDescription, tags []*elb.Tag) *Event {
	return &Event{
		VPCID:               *e.VPCId,
		ELBName:             e.LoadBalancerName,
		ELBDNSName:          e.DNSName,
		ELBListeners:        mapELBListeners(e.ListenerDescriptions),
		InstanceAWSIDs:      mapELBInstances(e.Instances),
		NetworkAWSIDs:       mapELBSubnets(e.Subnets),
		SecurityGroupAWSIDs: mapELBSecurityGroups(e.SecurityGroups),
		Tags:                mapELBTags(tags),
		//ELBIsPrivate: *e.,
	}
}
