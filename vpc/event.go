/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package vpc

import (
	"encoding/json"
	"errors"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/ernestio/ernestaws"
	"github.com/ernestio/ernestaws/credentials"
)

var (
	// ErrDatacenterIDInvalid ...
	ErrDatacenterIDInvalid = errors.New("Datacenter VPC ID invalid")
	// ErrDatacenterRegionInvalid ...
	ErrDatacenterRegionInvalid = errors.New("Datacenter Region invalid")
	// ErrDatacenterCredentialsInvalid ...
	ErrDatacenterCredentialsInvalid = errors.New("Datacenter credentials invalid")
)

// Event stores the template data
type Event struct {
	ProviderType     string            `json:"_provider"`
	ComponentType    string            `json:"_component"`
	ComponentID      string            `json:"_component_id"`
	State            string            `json:"_state"`
	Action           string            `json:"_action"`
	VpcID            *string           `json:"vpc_aws_id"`
	Name             string            `json:"name"`
	Subnet           *string           `json:"subnet"`
	AutoRemove       bool              `json:"auto_remove"`
	Tags             map[string]string `json:"tags"`
	DatacenterType   string            `json:"datacenter_type,omitempty"`
	DatacenterName   string            `json:"datacenter_name,omitempty"`
	DatacenterRegion string            `json:"datacenter_region"`
	AccessKeyID      string            `json:"aws_access_key_id"`
	SecretAccessKey  string            `json:"aws_secret_access_key"`
	Service          string            `json:"service"`
	ErrorMessage     string            `json:"error,omitempty"`
	Subject          string            `json:"-"`
	Body             []byte            `json:"-"`
	CryptoKey        string            `json:"-"`
}

// New : Constructor
func New(subject string, body []byte, cryptoKey string) ernestaws.Event {
	if strings.Split(subject, ".")[1] == "find" {
		return &Collection{Subject: subject, Body: body, CryptoKey: cryptoKey}
	}

	return &Event{Subject: subject, Body: body, CryptoKey: cryptoKey}
}

// GetBody : Gets the body for this event
func (ev *Event) GetBody() []byte {
	var err error
	if ev.Body, err = json.Marshal(ev); err != nil {
		log.Println(err.Error())
	}
	return ev.Body
}

// GetSubject : Gets the subject for this event
func (ev *Event) GetSubject() string {
	return ev.Subject
}

// Process : starts processing the current message
func (ev *Event) Process() (err error) {
	if err := json.Unmarshal(ev.Body, &ev); err != nil {
		ev.Error(err)
		return err
	}

	if err := ev.Validate(); err != nil {
		ev.Error(err)
		return err
	}

	return nil
}

// Error : Will respond the current event with an error
func (ev *Event) Error(err error) {
	log.Printf("Error: %s", err.Error())
	ev.ErrorMessage = err.Error()
	ev.State = "errored"

	ev.Body, err = json.Marshal(ev)
}

// Complete : sets the state of the event to completed
func (ev *Event) Complete() {
	ev.State = "completed"
}

// Validate checks if all criteria are met
func (ev *Event) Validate() error {
	if ev.Subject == "vpc.delete.aws" {
		if ev.VpcID == nil {
			return ErrDatacenterIDInvalid
		}
	}
	if ev.DatacenterRegion == "" {
		return ErrDatacenterRegionInvalid
	}

	if ev.AccessKeyID == "" || ev.SecretAccessKey == "" {
		return ErrDatacenterCredentialsInvalid
	}

	return nil
}

// Find : Find an object on aws
func (ev *Event) Find() error {
	return errors.New(ev.Subject + " not supported")
}

// Create : Creates a vpc object on aws
func (ev *Event) Create() error {
	svc := ev.getEC2Client()

	req := ec2.CreateVpcInput{
		CidrBlock: ev.Subnet,
	}
	resp, err := svc.CreateVpc(&req)
	if err != nil {
		return err
	}
	ev.VpcID = resp.Vpc.VpcId

	return ev.setTags()
}

// Update : Updates a vpc object on aws
func (ev *Event) Update() error {
	return errors.New(ev.Subject + " not supported")
}

// Delete : Deletes a vpc object on aws
func (ev *Event) Delete() error {
	svc := ev.getEC2Client()

	req := ec2.DeleteVpcInput{
		VpcId: ev.VpcID,
	}

	_, err := svc.DeleteVpc(&req)
	if err != nil {
		ev.ErrorMessage = "WARN : Could not remove the vpc - " + err.Error()
		return nil
	}

	return nil
}

// Get : Gets a vpc object on aws
func (ev *Event) Get() error {
	return errors.New(ev.Subject + " not supported")
}

func (ev *Event) getEC2Client() *ec2.EC2 {
	creds, _ := credentials.NewStaticCredentials(ev.AccessKeyID, ev.SecretAccessKey, ev.CryptoKey)
	return ec2.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})
}

func (ev *Event) setTags() error {
	svc := ev.getEC2Client()

	for key, val := range ev.Tags {
		req := &ec2.CreateTagsInput{
			Resources: []*string{ev.VpcID},
		}

		req.Tags = append(req.Tags, &ec2.Tag{
			Key:   &key,
			Value: &val,
		})

		_, err := svc.CreateTags(req)
		if err != nil {
			return err
		}
	}

	return nil
}

func mapTags(tags map[string]string) []*ec2.Tag {
	var t []*ec2.Tag

	for key, val := range tags {
		t = append(t, &ec2.Tag{
			Key:   &key,
			Value: &val,
		})
	}

	return t
}
