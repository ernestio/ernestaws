/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package nat

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
	ProviderType           string            `json:"_provider"`
	ComponentType          string            `json:"_component"`
	ComponentID            string            `json:"_component_id"`
	State                  string            `json:"_state"`
	Action                 string            `json:"_action"`
	NatGatewayAWSID        *string           `json:"nat_gateway_aws_id"`
	Name                   *string           `json:"name"`
	PublicNetwork          string            `json:"public_network"`
	RoutedNetworks         []string          `json:"routed_networks"`
	RoutedNetworkAWSIDs    []*string         `json:"routed_networks_aws_ids"`
	PublicNetworkAWSID     *string           `json:"public_network_aws_id"`
	NatGatewayAllocationID *string           `json:"nat_gateway_allocation_id"`
	NatGatewayAllocationIP *string           `json:"nat_gateway_allocation_ip"`
	InternetGatewayID      *string           `json:"internet_gateway_id"`
	DatacenterType         string            `json:"datacenter_type"`
	DatacenterName         string            `json:"datacenter_name"`
	DatacenterRegion       string            `json:"datacenter_region"`
	AccessKeyID            string            `json:"aws_access_key_id"`
	SecretAccessKey        string            `json:"aws_secret_access_key"`
	VpcID                  string            `json:"vpc_id"`
	Tags                   map[string]string `json:"tags"`
	Service                string            `json:"service"`
	ErrorMessage           string            `json:"error,omitempty"`
	Subject                string            `json:"-"`
	Body                   []byte            `json:"-"`
	CryptoKey              string            `json:"-"`
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
	if ev.DatacenterRegion == "" {
		return ErrDatacenterRegionInvalid
	}

	if ev.AccessKeyID == "" || ev.SecretAccessKey == "" {
		return ErrDatacenterCredentialsInvalid
	}

	if ev.Subject == "nat.delete.aws" {
		if ev.NatGatewayAWSID == nil {
			return ErrNatGatewayIDInvalid
		}
	} else {
		if ev.PublicNetworkAWSID == nil {
			return ErrNetworkIDInvalid
		}

		if len(ev.RoutedNetworkAWSIDs) < 1 {
			return ErrRoutedNetworksEmpty
		}
	}

	return nil
}

// Find : Find an object on aws
func (ev *Event) Find() error {
	return errors.New(ev.Subject + " not supported")
}

// Create : Creates a nat object on aws
func (ev *Event) Create() error {
	svc := ev.getEC2Client()

	// Create Elastic IP
	resp, err := svc.AllocateAddress(nil)
	if err != nil {
		return err
	}

	ev.NatGatewayAllocationID = resp.AllocationId
	ev.NatGatewayAllocationIP = resp.PublicIp

	// Create Internet Gateway
	ev.InternetGatewayID, err = ev.createInternetGateway(svc)
	if err != nil {
		return err
	}

	// Create Nat Gateway
	req := ec2.CreateNatGatewayInput{
		AllocationId: ev.NatGatewayAllocationID,
		SubnetId:     ev.PublicNetworkAWSID,
	}

	gwresp, err := svc.CreateNatGateway(&req)
	if err != nil {
		return err
	}

	ev.NatGatewayAWSID = gwresp.NatGateway.NatGatewayId

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

		err = ev.createNatGatewayRoutes(svc, rt, gwresp.NatGateway.NatGatewayId)
		if err != nil {
			return err
		}
	}

	return nil
}

// Update : Updates a nat object on aws
func (ev *Event) Update() error {
	svc := ev.getEC2Client()

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
	svc := ev.getEC2Client()

	req := ec2.DeleteNatGatewayInput{
		NatGatewayId: ev.NatGatewayAWSID,
	}

	_, err := svc.DeleteNatGateway(&req)
	if err != nil {
		return err
	}

	for ev.isNatGatewayDeleted(svc, ev.NatGatewayAWSID) == false {
		time.Sleep(time.Second * 3)
	}

	rreq := &ec2.ReleaseAddressInput{
		AllocationId: ev.NatGatewayAllocationID,
	}

	_, err = svc.ReleaseAddress(rreq)

	return err
}

// Get : Gets a nat object on aws
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

func (ev *Event) routingTableBySubnetID(svc *ec2.EC2, subnet *string) (*ec2.RouteTable, error) {
	f := []*ec2.Filter{
		&ec2.Filter{
			Name:   aws.String("association.subnet-id"),
			Values: []*string{subnet},
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

func (ev *Event) createInternetGateway(svc *ec2.EC2) (*string, error) {
	ig, err := ev.internetGatewayByVPCID(svc, ev.VpcID)
	if err != nil {
		return nil, err
	}

	if ig != nil {
		return ig.InternetGatewayId, nil
	}

	resp, err := svc.CreateInternetGateway(nil)
	if err != nil {
		return nil, err
	}

	req := ec2.AttachInternetGatewayInput{
		InternetGatewayId: resp.InternetGateway.InternetGatewayId,
		VpcId:             aws.String(ev.VpcID),
	}

	_, err = svc.AttachInternetGateway(&req)
	if err != nil {
		return nil, err
	}

	return resp.InternetGateway.InternetGatewayId, nil
}

func (ev *Event) createRouteTable(svc *ec2.EC2, subnet *string) (*ec2.RouteTable, error) {
	rt, err := ev.routingTableBySubnetID(svc, subnet)
	if err != nil {
		return nil, err
	}

	if rt != nil {
		return rt, nil
	}

	req := ec2.CreateRouteTableInput{
		VpcId: aws.String(ev.VpcID),
	}

	resp, err := svc.CreateRouteTable(&req)
	if err != nil {
		return nil, err
	}

	acreq := ec2.AssociateRouteTableInput{
		RouteTableId: resp.RouteTable.RouteTableId,
		SubnetId:     subnet,
	}

	_, err = svc.AssociateRouteTable(&acreq)
	if err != nil {
		return nil, err
	}

	return resp.RouteTable, nil
}

func (ev *Event) createNatGatewayRoutes(svc *ec2.EC2, rt *ec2.RouteTable, gwID *string) error {
	req := ec2.CreateRouteInput{
		RouteTableId:         rt.RouteTableId,
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		NatGatewayId:         gwID,
	}

	_, err := svc.CreateRoute(&req)
	if err != nil {
		return err
	}

	return nil
}

func (ev *Event) isNatGatewayDeleted(svc *ec2.EC2, id *string) bool {
	gw, _ := ev.natGatewayByID(svc, id)
	if gw == nil {
		return true
	}

	if *gw.State == ec2.NatGatewayStateDeleted {
		return true
	}

	return false
}

func (ev *Event) routeTableIsConfigured(rt *ec2.RouteTable) bool {
	gwID := ev.NatGatewayAWSID
	for _, route := range rt.Routes {
		if *route.DestinationCidrBlock == "0.0.0.0/0" && *route.NatGatewayId == *gwID {
			return true
		}
	}
	return false
}

func (ev *Event) natGatewayByID(svc *ec2.EC2, id *string) (*ec2.NatGateway, error) {
	req := ec2.DescribeNatGatewaysInput{
		NatGatewayIds: []*string{id},
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
