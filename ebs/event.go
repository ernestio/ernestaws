/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package ebs

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
	UUID               string            `json:"_uuid"`
	BatchID            string            `json:"_batch_id"`
	ProviderType       string            `json:"_type"`
	VPCID              string            `json:"vpc_id"`
	DatacenterRegion   string            `json:"datacenter_region"`
	AWSAccessKeyID     string            `json:"aws_access_key_id"`
	AWSSecretAccessKey string            `json:"aws_secret_access_key"`
	VolumeAWSID        *string           `json:"volume_aws_id"`
	Name               *string           `json:"name"`
	AvailabilityZone   *string           `json:"availability_zone"`
	VolumeType         *string           `json:"volume_type"`
	Size               *int64            `json:"size"`
	Iops               *int64            `json:"iops"`
	Encrypted          *bool             `json:"encrypted"`
	EncryptionKeyID    *string           `json:"encryption_key_id"`
	Tags               map[string]string `json:"tags"`
	ErrorMessage       string            `json:"error,omitempty"`
	Subject            string            `json:"-"`
	Body               []byte            `json:"-"`
	CryptoKey          string            `json:"-"`
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

	if ev.AWSAccessKeyID == "" || ev.AWSSecretAccessKey == "" {
		return ErrDatacenterCredentialsInvalid
	}

	if ev.Subject != "ebs_volume.create.aws" {
		if ev.VolumeAWSID == nil {
			return ErrVolumeIDInvalid
		}
	}

	if ev.Name == nil {
		return ErrVolumeNameInvalid
	}

	if ev.AvailabilityZone == nil {
		return ErrAvailabilityZoneInvalid
	}

	if ev.VolumeType == nil {
		return ErrVolumeTypeInvalid
	}

	return nil
}

// Find : Find an object on aws
func (ev *Event) Find() error {
	return errors.New(ev.Subject + " not supported")
}

// Create : Creates a instance object on aws
func (ev *Event) Create() error {
	svc := ev.getEC2Client()

	req := &ec2.CreateVolumeInput{
		AvailabilityZone: ev.AvailabilityZone,
		VolumeType:       ev.VolumeType,
		Size:             ev.Size,
		Iops:             ev.Iops,
		Encrypted:        ev.Encrypted,
		KmsKeyId:         ev.EncryptionKeyID,
	}

	resp, err := svc.CreateVolume(req)
	if err != nil {
		return err
	}

	ev.VolumeAWSID = resp.VolumeId

	return ev.setTags()
}

// Update : Updates a instance object on aws
func (ev *Event) Update() error {
	return errors.New(ev.Subject + " not supported")
}

// Delete : Deletes a instance object on aws
func (ev *Event) Delete() error {
	svc := ev.getEC2Client()

	req := &ec2.DeleteVolumeInput{
		VolumeId: ev.VolumeAWSID,
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

func (ev *Event) getEC2Client() *ec2.EC2 {
	creds, _ := credentials.NewStaticCredentials(ev.AWSAccessKeyID, ev.AWSSecretAccessKey, ev.CryptoKey)
	return ec2.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})
}

func (ev *Event) setTags() error {
	svc := ev.getEC2Client()

	for key, val := range ev.Tags {
		req := &ec2.CreateTagsInput{
			Resources: []*string{ev.VolumeAWSID},
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
