/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package internetgateway

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
	// ErrInternetGatewayAWSIDInvalid ...
	ErrInternetGatewayAWSIDInvalid = errors.New("Internet Gateway ID invalid")
)

// Event stores the network data
type Event struct {
	ProviderType         string            `json:"_provider"`
	ComponentType        string            `json:"_component"`
	ComponentID          string            `json:"_component_id"`
	State                string            `json:"_state"`
	Action               string            `json:"_action"`
	InternetGatewayAWSID *string           `json:"internet_gateway_aws_id"`
	Name                 *string           `json:"name"`
	Tags                 map[string]string `json:"tags"`
	DatacenterType       string            `json:"datacenter_type"`
	DatacenterName       string            `json:"datacenter_name"`
	DatacenterRegion     string            `json:"datacenter_region"`
	AccessKeyID          string            `json:"aws_access_key_id"`
	SecretAccessKey      string            `json:"aws_secret_access_key"`
	Vpc                  string            `json:"vpc"`
	VpcID                string            `json:"vpc_id"`
	Service              string            `json:"service"`
	ErrorMessage         string            `json:"error,omitempty"`
	Subject              string            `json:"-"`
	Body                 []byte            `json:"-"`
	CryptoKey            string            `json:"-"`
}

// New : Constructor
func New(subject string, body []byte, cryptoKey string) ernestaws.Event {
	if strings.Split(subject, ".")[1] == "find" {
		return &Collection{Subject: subject, Body: body, CryptoKey: cryptoKey}
	}

	return &Event{Subject: subject, Body: body, CryptoKey: cryptoKey}
}

// Validate checks if all criteria are met
func (ev *Event) Validate() error {
	if ev.VpcID == "" {
		return ErrDatacenterIDInvalid
	}

	if ev.DatacenterRegion == "" {
		return ErrDatacenterRegionInvalid
	}

	if ev.AccessKeyID == "" || ev.SecretAccessKey == "" {
		return ErrDatacenterCredentialsInvalid
	}

	if ev.Subject == "internet_gateway.delete.aws" {
		if ev.InternetGatewayAWSID == nil {
			return ErrInternetGatewayAWSIDInvalid
		}
	}

	return nil
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

// Find : Find an object on aws
func (ev *Event) Find() error {
	return errors.New(ev.Subject + " not supported")
}

// Create : Creates a nat object on aws
func (ev *Event) Create() error {
	svc := ev.getEC2Client()

	ig, err := ev.internetGatewayByVPCID(svc, ev.VpcID)
	if err != nil {
		return err
	}

	if ig != nil {
		ev.InternetGatewayAWSID = ig.InternetGatewayId
		return nil
	}

	resp, err := svc.CreateInternetGateway(nil)
	if err != nil {
		return err
	}

	req := ec2.AttachInternetGatewayInput{
		InternetGatewayId: resp.InternetGateway.InternetGatewayId,
		VpcId:             aws.String(ev.VpcID),
	}

	_, err = svc.AttachInternetGateway(&req)
	if err != nil {
		return err
	}

	ev.InternetGatewayAWSID = resp.InternetGateway.InternetGatewayId

	return ev.setTags()
}

// Update : Updates a nat object on aws
func (ev *Event) Update() error {
	return errors.New(ev.Subject + " not supported")
}

// Delete : Deletes a nat object on aws
func (ev *Event) Delete() error {
	svc := ev.getEC2Client()

	err := ev.deleteRouteTables()
	if err != nil {
		return err
	}

	req := &ec2.DeleteInternetGatewayInput{
		InternetGatewayId: ev.InternetGatewayAWSID,
	}

	_, err = svc.DeleteInternetGateway(req)

	return err
}

// Get : Gets a nat object on aws
func (ev *Event) Get() error {
	return errors.New(ev.Subject + " not supported")
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

func (ev *Event) getEC2Client() *ec2.EC2 {
	creds, _ := credentials.NewStaticCredentials(ev.AccessKeyID, ev.SecretAccessKey, ev.CryptoKey)
	return ec2.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})
}

func (ev *Event) internetGatewayByVPCID(svc *ec2.EC2, vpc string) (*ec2.InternetGateway, error) {
	f := []*ec2.Filter{
		&ec2.Filter{
			Name:   aws.String("attachment.vpc-id"),
			Values: []*string{aws.String(vpc)},
		},
	}

	req := ec2.DescribeInternetGatewaysInput{
		Filters: f,
	}

	resp, err := svc.DescribeInternetGateways(&req)
	if err != nil {
		return nil, err
	}

	if len(resp.InternetGateways) == 0 {
		return nil, nil
	}

	return resp.InternetGateways[0], nil
}

func (ev *Event) deleteRouteTables() error {
	svc := ev.getEC2Client()

	f := []*ec2.Filter{
		&ec2.Filter{
			Name:   aws.String("route.gateway-id"),
			Values: []*string{ev.InternetGatewayAWSID},
		},
	}

	req := &ec2.DescribeRouteTablesInput{
		Filters: f,
	}

	resp, err := svc.DescribeRouteTables(req)
	if err != nil {
		return err
	}

	for _, rt := range resp.RouteTables {
		for _, assoc := range rt.Associations {
			ddreq := &ec2.DisassociateRouteTableInput{
				AssociationId: assoc.RouteTableAssociationId,
			}

			_, err = svc.DisassociateRouteTable(ddreq)
			if err != nil {
				return err
			}
		}

		dreq := &ec2.DeleteRouteTableInput{
			RouteTableId: rt.RouteTableId,
		}

		_, err = svc.DeleteRouteTable(dreq)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ev *Event) setTags() error {
	svc := ev.getEC2Client()

	for key, val := range ev.Tags {
		req := &ec2.CreateTagsInput{
			Resources: []*string{ev.InternetGatewayAWSID},
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
