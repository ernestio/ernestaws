/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package ebs

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
	// ErrVolumeIDInvalid ...
	ErrVolumeIDInvalid = errors.New("EBS volume aws id invalid")
	// ErrAvailabilityZoneInvalid ...
	ErrAvailabilityZoneInvalid = errors.New("Availability zone invalid")
	// ErrVolumeNameInvalid ...
	ErrVolumeNameInvalid = errors.New("EBS volume name invalid")
	// ErrVolumeTypeInvalid ...
	ErrVolumeTypeInvalid = errors.New("EBS volume type invalid")
)

// Event stores the template data
type Event struct {
	UUID                  string `json:"_uuid"`
	BatchID               string `json:"_batch_id"`
	ProviderType          string `json:"_type"`
	VPCID                 string `json:"vpc_id"`
	DatacenterRegion      string `json:"datacenter_region"`
	DatacenterAccessKey   string `json:"datacenter_secret"`
	DatacenterAccessToken string `json:"datacenter_token"`
	VolumeAWSID           string `json:"volume_aws_id"`
	Name                  string `json:"name"`
	AvailabilityZone      string `json:"availability_zone"`
	VolumeType            string `json:"volume_type"`
	Size                  *int64 `json:"size"`
	Iops                  *int64 `json:"iops"`
	Encrypted             bool   `json:"encrypted"`
	EncryptionKeyID       string `json:"encryption_key_id"`
	ErrorMessage          string `json:"error,omitempty"`
	Subject               string `json:"-"`
	Body                  []byte `json:"-"`
	CryptoKey             string `json:"-"`
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
	if ev.VPCID == "" {
		return ErrDatacenterIDInvalid
	}

	if ev.DatacenterRegion == "" {
		return ErrDatacenterRegionInvalid
	}

	if ev.DatacenterAccessKey == "" || ev.DatacenterAccessToken == "" {
		return ErrDatacenterCredentialsInvalid
	}

	if ev.Subject != "ebs_volume.create.aws" {
		if ev.VolumeAWSID == "" {
			return ErrVolumeIDInvalid
		}
	}

	if ev.Name == "" {
		return ErrVolumeNameInvalid
	}

	if ev.AvailabilityZone == "" {
		return ErrAvailabilityZoneInvalid
	}

	if ev.VolumeType == "" {
		return ErrVolumeTypeInvalid
	}

	return nil
}

// Create : Creates a instance object on aws
func (ev *Event) Create() error {
	creds, _ := credentials.NewStaticCredentials(ev.DatacenterAccessKey, ev.DatacenterAccessToken, ev.CryptoKey)
	svc := ec2.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})

	req := &ec2.CreateVolumeInput{
		AvailabilityZone: aws.String(ev.AvailabilityZone),
		VolumeType:       aws.String(ev.VolumeType),
		Size:             ev.Size,
		Iops:             ev.Iops,
		Encrypted:        aws.Bool(ev.Encrypted),
		KmsKeyId:         aws.String(ev.EncryptionKeyID),
	}

	resp, err := svc.CreateVolume(req)
	if err != nil {
		return err
	}

	ev.VolumeAWSID = *resp.VolumeId

	return nil
}

// Update : Updates a instance object on aws
func (ev *Event) Update() error {
	return errors.New(ev.Subject + " not supported")
}

// Delete : Deletes a instance object on aws
func (ev *Event) Delete() error {
	creds, _ := credentials.NewStaticCredentials(ev.DatacenterAccessKey, ev.DatacenterAccessToken, ev.CryptoKey)
	svc := ec2.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})

	req := &ec2.DeleteVolumeInput{
		VolumeId: aws.String(ev.VolumeAWSID),
	}

	_, err := svc.DeleteVolume(req)
	if err != nil {
		return err
	}

	return nil
}

// Get : Gets a instance object on aws
func (ev *Event) Get() error {
	return errors.New(ev.Subject + " not supported")
}
