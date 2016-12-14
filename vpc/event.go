/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package vpc

import (
	"encoding/json"
	"errors"
	"log"

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
	UUID               string `json:"_uuid"`
	BatchID            string `json:"_batch_id"`
	ProviderType       string `json:"_type"`
	DatacenterName     string `json:"datacenter_name"`
	DatacenterRegion   string `json:"datacenter_region"`
	AWSAccessKeyID     string `json:"aws_access_key_id"`
	AWSSecretAccessKey string `json:"aws_secret_access_key"`
	VpcID              string `json:"vpc_id"`
	VpcSubnet          string `json:"vpc_subnet"`
	ErrorMessage       string `json:"error,omitempty"`
	Subject            string `json:"-"`
	Body               []byte `json:"-"`
	CryptoKey          string `json:"-"`
}

// New : Constructor
func New(subject string, body []byte, cryptoKey string) ernestaws.Event {
	n := Event{Subject: subject, Body: body, CryptoKey: cryptoKey}

	return &n
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

	if ev.AWSAccessKeyID == "" || ev.AWSSecretAccessKey == "" {
		return ErrDatacenterCredentialsInvalid
	}

	return nil
}

// Create : Creates a vpc object on aws
func (ev *Event) Create() error {
	svc := ev.getEC2Client()

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
	svc := ev.getEC2Client()

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

func (ev *Event) getEC2Client() *ec2.EC2 {
	creds, _ := credentials.NewStaticCredentials(ev.AWSAccessKeyID, ev.AWSSecretAccessKey, ev.CryptoKey)
	return ec2.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})
}
