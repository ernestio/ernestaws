/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package firewall

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

// Find : Find security groups on aws
func (col *Collection) Find() error {
	svc := col.getEC2Client()

	req := &ec2.DescribeSecurityGroupsInput{
		Filters: mapFilters(col.Tags),
	}

	resp, err := svc.DescribeSecurityGroups(req)
	if err != nil {
		return err
	}

	for _, sg := range resp.SecurityGroups {
		col.Results = append(col.Results, toEvent(sg))
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

	for key, val := range tags {
		f = append(f, &ec2.Filter{
			Name:   aws.String("tag:" + key),
			Values: []*string{aws.String(val)},
		})
	}

	return f
}

func mapEC2Tags(input []*ec2.Tag) map[string]string {
	t := make(map[string]string)

	for _, tag := range input {
		t[*tag.Key] = *tag.Value
	}

	return t
}

// toEvent converts an ec2 security group object to an ernest event
func toEvent(sg *ec2.SecurityGroup) *Event {
	e := &Event{
		VPCID:              *sg.VpcId,
		SecurityGroupAWSID: sg.GroupId,
		SecurityGroupName:  sg.GroupName,
		Tags:               mapEC2Tags(sg.Tags),
	}

	e.SecurityGroupRules.Ingress = mapSecurityGroupRules(sg.IpPermissions)
	e.SecurityGroupRules.Egress = mapSecurityGroupRules(sg.IpPermissionsEgress)
	return e
}

func mapSecurityGroupRules(perms []*ec2.IpPermission) []rule {
	var rules []rule

	for _, p := range perms {
		for _, r := range p.IpRanges {
			rules = append(rules, rule{
				IP:       r.CidrIp,
				Protocol: p.IpProtocol,
				FromPort: p.FromPort,
				ToPort:   p.ToPort,
			})
		}
	}

	return rules
}
