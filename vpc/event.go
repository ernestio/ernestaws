/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package vpc

import (
	"encoding/json"
	"errors"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/ernestio/ernestaws"
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
	UUID                  string `json:"_uuid"`
	BatchID               string `json:"_batch_id"`
	ProviderType          string `json:"_type"`
	DatacenterName        string `json:"datacenter_name"`
	DatacenterRegion      string `json:"datacenter_region"`
	DatacenterAccessKey   string `json:"datacenter_access_key"`
	DatacenterAccessToken string `json:"datacenter_access_token"`
	VpcID                 string `json:"vpc_id"`
	VpcSubnet             string `json:"vpc_subnet"`
	ErrorMessage          string `json:"error_message,omitempty"`
	Subject               string `json:"-"`
	Body                  []byte `json:"-"`
}

// New : Constructor
func New(subject string, body []byte) ernestaws.Event {
	n := Event{Subject: subject, Body: body}

	return &n
}

// GetBody : Gets the body for this event
func (ev *Event) GetBody() []byte {
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

	ev.Body, err = json.Marshal(ev)
}

// Validate checks if all criteria are met
func (ev *Event) Validate() error {
	if ev.Subject == "vpc.delete.aws" {
		if ev.VpcID == "" {
			return ErrDatacenterIDInvalid
		}
	}
	if ev.DatacenterRegion == "" {
		return ErrDatacenterRegionInvalid
	}

	if ev.DatacenterAccessKey == "" || ev.DatacenterAccessToken == "" {
		return ErrDatacenterCredentialsInvalid
	}

	return nil
}

// Create : Creates a vpc object on aws
func (ev *Event) Create() error {
	creds := credentials.NewStaticCredentials(ev.DatacenterAccessKey, ev.DatacenterAccessToken, "")
	svc := ec2.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})

	req := ec2.CreateVpcInput{
		CidrBlock: aws.String(ev.VpcSubnet),
	}
	resp, err := svc.CreateVpc(&req)
	if err != nil {
		return err
	}
	ev.VpcID = *resp.Vpc.VpcId

	return nil
}

// Update : Updates a vpc object on aws
func (ev *Event) Update() error {
	return errors.New(ev.Subject + " not supported")
}

// Delete : Deletes a vpc object on aws
func (ev *Event) Delete() error {
	creds := credentials.NewStaticCredentials(ev.DatacenterAccessKey, ev.DatacenterAccessToken, "")
	svc := ec2.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})

	req := ec2.DeleteVpcInput{
		VpcId: aws.String(ev.VpcID),
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
