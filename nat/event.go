/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package nat

import (
	"encoding/json"
	"errors"
	"log"
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
	// ErrDatacenterCredentialsInvalid ..
	ErrDatacenterCredentialsInvalid = errors.New("Datacenter credentials invalid")
	// ErrNetworkIDInvalid ...
	ErrNetworkIDInvalid = errors.New("Network id invalid")
	// ErrRoutedNetworksEmpty ...
	ErrRoutedNetworksEmpty = errors.New("Routed networks are empty")
	// ErrNatGatewayIDInvalid ...
	ErrNatGatewayIDInvalid = errors.New("Nat Gateway aws id invalid")
)

// Event stores the nat data
type Event struct {
	UUID                   string   `json:"_uuid"`
	BatchID                string   `json:"_batch_id"`
	ProviderType           string   `json:"_type"`
	VPCID                  string   `json:"vpc_id"`
	DatacenterRegion       string   `json:"datacenter_region"`
	DatacenterAccessKey    string   `json:"datacenter_secret"`
	DatacenterAccessToken  string   `json:"datacenter_token"`
	NetworkAWSID           string   `json:"network_aws_id"`
	PublicNetwork          string   `json:"public_network"`
	PublicNetworkAWSID     string   `json:"public_network_aws_id"`
	RoutedNetworks         []string `json:"routed_networks"`
	RoutedNetworkAWSIDs    []string `json:"routed_networks_aws_ids"`
	NatGatewayAWSID        string   `json:"nat_gateway_aws_id"`
	NatGatewayAllocationID string   `json:"nat_gateway_allocation_id"`
	NatGatewayAllocationIP string   `json:"nat_gateway_allocation_ip"`
	InternetGatewayID      string   `json:"internet_gateway_id"`
	ErrorMessage           string   `json:"error,omitempty"`
	Subject                string   `json:"-"`
	Body                   []byte   `json:"-"`
	CryptoKey              string   `json:"-"`
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

	if ev.Subject == "nat.delete.aws" {
		if ev.NatGatewayAWSID == "" {
			return ErrNatGatewayIDInvalid
		}
	} else {
		if ev.PublicNetworkAWSID == "" {
			return ErrNetworkIDInvalid
		}

		if len(ev.RoutedNetworkAWSIDs) < 1 {
			return ErrRoutedNetworksEmpty
		}
	}

	return nil
}

// Create : Creates a nat object on aws
func (ev *Event) Create() error {
	creds, _ := credentials.NewStaticCredentials(ev.DatacenterAccessKey, ev.DatacenterAccessToken, ev.CryptoKey)
	svc := ec2.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})

	// Create Elastic IP
	resp, err := svc.AllocateAddress(nil)
	if err != nil {
		return err
	}

	ev.NatGatewayAllocationID = *resp.AllocationId
	ev.NatGatewayAllocationIP = *resp.PublicIp

	// Create Internet Gateway
	ev.InternetGatewayID, err = ev.createInternetGateway(svc)
	if err != nil {
		return err
	}

	// Create Nat Gateway
	req := ec2.CreateNatGatewayInput{
		AllocationId: aws.String(ev.NatGatewayAllocationID),
		SubnetId:     aws.String(ev.PublicNetworkAWSID),
	}

	gwresp, err := svc.CreateNatGateway(&req)
	if err != nil {
		return err
	}

	ev.NatGatewayAWSID = *gwresp.NatGateway.NatGatewayId

	waitnat := ec2.DescribeNatGatewaysInput{
		NatGatewayIds: []*string{gwresp.NatGateway.NatGatewayId},
	}

	err = svc.WaitUntilNatGatewayAvailable(&waitnat)
	if err != nil {
		return err
	}

	for _, networkID := range ev.RoutedNetworkAWSIDs {
		rt, err := ev.createRouteTable(svc, networkID)
		if err != nil {
			return err
		}

		err = ev.createNatGatewayRoutes(svc, rt, *gwresp.NatGateway.NatGatewayId)
		if err != nil {
			return err
		}
	}

	return nil
}

// Update : Updates a nat object on aws
func (ev *Event) Update() error {
	creds, _ := credentials.NewStaticCredentials(ev.DatacenterAccessKey, ev.DatacenterAccessToken, ev.CryptoKey)
	svc := ec2.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})

	for _, networkID := range ev.RoutedNetworkAWSIDs {
		rt, err := ev.createRouteTable(svc, networkID)
		if err != nil {
			return err
		}

		if ev.routeTableIsConfigured(rt) {
			continue
		}

		err = ev.createNatGatewayRoutes(svc, rt, ev.NatGatewayAWSID)
		if err != nil {
			return err
		}
	}

	return nil
}

// Delete : Deletes a nat object on aws
func (ev *Event) Delete() error {
	creds, _ := credentials.NewStaticCredentials(ev.DatacenterAccessKey, ev.DatacenterAccessToken, ev.CryptoKey)
	svc := ec2.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})

	req := ec2.DeleteNatGatewayInput{
		NatGatewayId: aws.String(ev.NatGatewayAWSID),
	}

	_, err := svc.DeleteNatGateway(&req)
	if err != nil {
		return err
	}

	for ev.isNatGatewayDeleted(svc, ev.NatGatewayAWSID) == false {
		time.Sleep(time.Second * 3)
	}

	dreq := &ec2.DisassociateAddressInput{
		AssociationId: aws.String(ev.NatGatewayAllocationID),
	}

	_, err = svc.DisassociateAddress(dreq)

	return err
}

// Get : Gets a nat object on aws
func (ev *Event) Get() error {
	return errors.New(ev.Subject + " not supported")
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

func (ev *Event) createInternetGateway(svc *ec2.EC2) (string, error) {
	ig, err := ev.internetGatewayByVPCID(svc, ev.VPCID)
	if err != nil {
		return "", err
	}

	if ig != nil {
		return *ig.InternetGatewayId, nil
	}

	resp, err := svc.CreateInternetGateway(nil)
	if err != nil {
		return "", err
	}

	req := ec2.AttachInternetGatewayInput{
		InternetGatewayId: resp.InternetGateway.InternetGatewayId,
		VpcId:             aws.String(ev.VPCID),
	}

	_, err = svc.AttachInternetGateway(&req)
	if err != nil {
		return "", err
	}

	return *resp.InternetGateway.InternetGatewayId, nil
}

func (ev *Event) createRouteTable(svc *ec2.EC2, subnet string) (*ec2.RouteTable, error) {
	rt, err := ev.routingTableBySubnetID(svc, subnet)
	if err != nil {
		return nil, err
	}

	if rt != nil {
		return rt, nil
	}

	req := ec2.CreateRouteTableInput{
		VpcId: aws.String(ev.VPCID),
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

func (ev *Event) createNatGatewayRoutes(svc *ec2.EC2, rt *ec2.RouteTable, gwID string) error {
	req := ec2.CreateRouteInput{
		RouteTableId:         rt.RouteTableId,
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		NatGatewayId:         aws.String(gwID),
	}

	_, err := svc.CreateRoute(&req)
	if err != nil {
		return err
	}

	return nil
}

func (ev *Event) isNatGatewayDeleted(svc *ec2.EC2, id string) bool {
	gw, _ := ev.natGatewayByID(svc, id)
	if *gw.State == ec2.NatGatewayStateDeleted {
		return true
	}

	return false
}

func (ev *Event) routeTableIsConfigured(rt *ec2.RouteTable) bool {
	gwID := ev.NatGatewayAWSID
	for _, route := range rt.Routes {
		if *route.DestinationCidrBlock == "0.0.0.0/0" && *route.NatGatewayId == gwID {
			return true
		}
	}
	return false
}

func (ev *Event) natGatewayByID(svc *ec2.EC2, id string) (*ec2.NatGateway, error) {
	req := ec2.DescribeNatGatewaysInput{
		NatGatewayIds: []*string{aws.String(id)},
	}
	resp, err := svc.DescribeNatGateways(&req)
	if err != nil {
		return nil, err
	}

	if len(resp.NatGateways) != 1 {
		return nil, errors.New("Could not find nat gateway")
	}

	return resp.NatGateways[0], nil
}
