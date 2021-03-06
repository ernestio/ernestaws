/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package network

import (
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

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
	// ErrNetworkSubnetInvalid ...
	ErrNetworkSubnetInvalid = errors.New("Network subnet invalid")
	// ErrNetworkAWSIDInvalid ...
	ErrNetworkAWSIDInvalid = errors.New("Network aws id invalid")
)

// Event stores the network data
type Event struct {
	ProviderType         string            `json:"_provider"`
	ComponentType        string            `json:"_component"`
	ComponentID          string            `json:"_component_id"`
	State                string            `json:"_state"`
	Action               string            `json:"_action"`
	NetworkAWSID         *string           `json:"network_aws_id"`
	Name                 *string           `json:"name"`
	Subnet               *string           `json:"range"`
	IsPublic             *bool             `json:"is_public"`
	InternetGateway      string            `json:"internet_gateway"`
	InternetGatewayAWSID string            `json:"internet_gateway_aws_id"`
	AvailabilityZone     *string           `json:"availability_zone"`
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

	if ev.Subject == "network.delete.aws" {
		if ev.NetworkAWSID == nil {
			return ErrNetworkAWSIDInvalid
		}
	} else {
		if ev.Subnet == nil {
			return ErrNetworkSubnetInvalid
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

	req := ec2.CreateSubnetInput{
		VpcId:            aws.String(ev.VpcID),
		CidrBlock:        ev.Subnet,
		AvailabilityZone: ev.AvailabilityZone,
	}

	resp, err := svc.CreateSubnet(&req)
	if err != nil {
		return err
	}

	if *ev.IsPublic {
		// Create Internet Gateway
		gateway, err := ev.createInternetGateway(svc, ev.VpcID)
		if err != nil {
			return err
		}

		// Create Route Table and direct traffic to Internet Gateway
		rt, err := ev.createRouteTable(svc, ev.VpcID, *resp.Subnet.SubnetId)
		if err != nil {
			return err
		}

		err = ev.createGatewayRoutes(svc, rt, gateway)
		if err != nil {
			return err
		}

		// Modify subnet to assign public IP's on launch
		mod := ec2.ModifySubnetAttributeInput{
			SubnetId:            resp.Subnet.SubnetId,
			MapPublicIpOnLaunch: &ec2.AttributeBooleanValue{Value: aws.Bool(true)},
		}

		_, err = svc.ModifySubnetAttribute(&mod)
		if err != nil {
			return err
		}
	}

	ev.NetworkAWSID = resp.Subnet.SubnetId
	ev.AvailabilityZone = resp.Subnet.AvailabilityZone

	return ev.setTags()
}

// Update : Updates a nat object on aws
func (ev *Event) Update() error {
	return errors.New(ev.Subject + " not supported")
}

// Delete : Deletes a nat object on aws
func (ev *Event) Delete() error {
	svc := ev.getEC2Client()

	err := ev.waitForInterfaceRemoval(svc, ev.NetworkAWSID)
	if err != nil {
		return err
	}

	req := ec2.DeleteSubnetInput{
		SubnetId: ev.NetworkAWSID,
	}

	_, err = svc.DeleteSubnet(&req)

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

func (ev *Event) routingTableBySubnetID(svc *ec2.EC2, subnet string) (*ec2.RouteTable, error) {
	f := []*ec2.Filter{
		&ec2.Filter{
			Name:   aws.String("association.subnet-id"),
			Values: []*string{aws.String(subnet)},
		},
	}

	req := ec2.DescribeRouteTablesInput{
		Filters: f,
	}

	resp, err := svc.DescribeRouteTables(&req)
	if err != nil {
		return nil, err
	}

	if len(resp.RouteTables) == 0 {
		return nil, nil
	}

	return resp.RouteTables[0], nil
}

func (ev *Event) createInternetGateway(svc *ec2.EC2, vpc string) (*ec2.InternetGateway, error) {
	ig, err := ev.internetGatewayByVPCID(svc, vpc)
	if err != nil {
		return nil, err
	}

	if ig != nil {
		return ig, nil
	}

	resp, err := svc.CreateInternetGateway(nil)
	if err != nil {
		return nil, err
	}

	req := ec2.AttachInternetGatewayInput{
		InternetGatewayId: resp.InternetGateway.InternetGatewayId,
		VpcId:             aws.String(vpc),
	}

	_, err = svc.AttachInternetGateway(&req)
	if err != nil {
		return nil, err
	}

	return resp.InternetGateway, nil
}

func (ev *Event) createRouteTable(svc *ec2.EC2, vpc, subnet string) (*ec2.RouteTable, error) {
	rt, err := ev.routingTableBySubnetID(svc, subnet)
	if err != nil {
		return nil, err
	}

	if rt != nil {
		return rt, nil
	}

	req := ec2.CreateRouteTableInput{
		VpcId: aws.String(vpc),
	}

	resp, err := svc.CreateRouteTable(&req)
	if err != nil {
		return nil, err
	}

	acreq := ec2.AssociateRouteTableInput{
		RouteTableId: resp.RouteTable.RouteTableId,
		SubnetId:     aws.String(subnet),
	}

	_, err = svc.AssociateRouteTable(&acreq)
	if err != nil {
		return nil, err
	}

	return resp.RouteTable, nil
}

func (ev *Event) createGatewayRoutes(svc *ec2.EC2, rt *ec2.RouteTable, gw *ec2.InternetGateway) error {
	req := ec2.CreateRouteInput{
		RouteTableId:         rt.RouteTableId,
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		GatewayId:            gw.InternetGatewayId,
	}

	_, err := svc.CreateRoute(&req)
	if err != nil {
		return err
	}

	return nil
}

func (ev *Event) waitForInterfaceRemoval(svc *ec2.EC2, networkID *string) error {
	for {
		resp, err := ev.getNetworkInterfaces(svc, networkID)
		if err != nil {
			return err
		}

		if len(resp.NetworkInterfaces) == 0 {
			return nil
		}

		time.Sleep(time.Second)
	}
}

func (ev *Event) getNetworkInterfaces(svc *ec2.EC2, networkID *string) (*ec2.DescribeNetworkInterfacesOutput, error) {
	f := []*ec2.Filter{
		&ec2.Filter{
			Name:   aws.String("subnet-id"),
			Values: []*string{networkID},
		},
	}

	req := ec2.DescribeNetworkInterfacesInput{
		Filters: f,
	}

	return svc.DescribeNetworkInterfaces(&req)
}

func (ev *Event) setTags() error {
	svc := ev.getEC2Client()

	for key, val := range ev.Tags {
		req := &ec2.CreateTagsInput{
			Resources: []*string{ev.NetworkAWSID},
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
