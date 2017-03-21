/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package instance

import (
	"encoding/json"
	"errors"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/ernestio/ernestaws/credentials"
)

// Collection ....
type Collection struct {
	ProviderType       string            `json:"_provider"`
	ComponentType      string            `json:"_component"`
	ComponentID        string            `json:"_component_id"`
	State              string            `json:"_state"`
	Action             string            `json:"_action"`
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
	col.State = "errored"

	col.Body, err = json.Marshal(col)
}

// Complete : sets the state of the event to completed
func (col *Collection) Complete() {
	col.State = "completed"
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

// Find : Find instances on aws
func (col *Collection) Find() error {
	svc := col.getEC2Client()

	req := &ec2.DescribeInstancesInput{
		Filters: mapFilters(col.Tags),
	}

	resp, err := svc.DescribeInstances(req)
	if err != nil {
		return err
	}

	for _, r := range resp.Reservations {
		for _, i := range r.Instances {
			col.Results = append(col.Results, toEvent(i))
		}
	}

	return nil
}

func (col *Collection) getEC2Client() *ec2.EC2 {
	creds, _ := credentials.NewStaticCredentials(col.AWSAccessKeyID, col.AWSSecretAccessKey, col.CryptoKey)
	return ec2.New(session.New(), &aws.Config{
		Region:      aws.String(col.DatacenterRegion),
		Credentials: creds,
	})
}

func mapFilters(tags map[string]string) []*ec2.Filter {
	var f []*ec2.Filter

	f = append(f, &ec2.Filter{
		Name:   aws.String("instance-state-name"),
		Values: []*string{aws.String("running"), aws.String("stopped")},
	})

	for key, val := range tags {
		f = append(f, &ec2.Filter{
			Name:   aws.String("tag:" + key),
			Values: []*string{aws.String(val)},
		})
	}

	return f
}

func mapAWSSecurityGroupIDs(gi []*ec2.GroupIdentifier) []*string {
	var sgs []*string

	for _, sg := range gi {
		sgs = append(sgs, sg.GroupId)
	}

	return sgs
}

func mapAWSVolumes(vs []*ec2.InstanceBlockDeviceMapping, rootDevice *string) []Volume {
	var vols []Volume

	for _, v := range vs {
		// omit root disk!
		if *v.DeviceName != *rootDevice {
			vols = append(vols, Volume{
				Device:      v.DeviceName,
				VolumeAWSID: v.Ebs.VolumeId,
			})
		}
	}

	return vols
}

// ToEvent converts an ec2 instance object to an ernest event
func toEvent(i *ec2.Instance) *Event {
	tags := mapEC2Tags(i.Tags)
	name := tags["Name"]

	return &Event{
		ProviderType:        "aws",
		ComponentType:       "instance",
		ComponentID:         "instance::" + name,
		InstanceAWSID:       i.InstanceId,
		Name:                aws.String(name),
		Type:                i.InstanceType,
		Image:               i.ImageId,
		NetworkAWSID:        i.SubnetId,
		SecurityGroupAWSIDs: mapAWSSecurityGroupIDs(i.SecurityGroups),
		IP:                  i.PrivateIpAddress,
		KeyPair:             i.KeyName,
		PublicIP:            i.PublicIpAddress,
		Volumes:             mapAWSVolumes(i.BlockDeviceMappings, i.RootDeviceName),
		Tags:                tags,
	}
}
