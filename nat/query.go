/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package nat

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

// Find : Find nat gateways on aws
func (col *Collection) Find() error {
	svc := col.getEC2Client()

	resp, err := svc.DescribeNatGateways(nil)
	if err != nil {
		return err
	}

	// As tags aren't supported on nat gw's, we append all items to be filtered later.
	for _, ng := range resp.NatGateways {
		var name string

		networks, err := col.getRoutedNetworks(ng.NatGatewayId)
		if err != nil {
			return err
		}

		for _, network := range networks {
			name, err = col.getGatewayName(network)
			if err != nil {
				return err
			}

			if name != "" {
				break
			}
		}

		e := toEvent(ng, name)
		e.RoutedNetworkAWSIDs = networks

		if name != "" {
			col.Results = append(col.Results, e)
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

	for key, val := range tags {
		f = append(f, &ec2.Filter{
			Name:   aws.String("tag:" + key),
			Values: []*string{aws.String(val)},
		})
	}

	return f
}

func getPublicAllocation(addresses []*ec2.NatGatewayAddress) (*string, *string) {
	for _, a := range addresses {
		if a.PublicIp != nil {
			return a.AllocationId, a.PublicIp
		}
	}
	return nil, nil
}

func (col *Collection) getRoutedNetworks(gatewayID *string) ([]*string, error) {
	var f []*ec2.Filter
	var ids []*string

	svc := col.getEC2Client()

	f = append(f, &ec2.Filter{
		Name:   aws.String("route.nat-gateway-id"),
		Values: []*string{gatewayID},
	})

	req := &ec2.DescribeRouteTablesInput{
		Filters: f,
	}

	resp, err := svc.DescribeRouteTables(req)
	if err != nil {
		return ids, err
	}

	for _, rt := range resp.RouteTables {
		for _, assoc := range rt.Associations {
			if assoc.SubnetId != nil {
				ids = append(ids, assoc.SubnetId)
			}
		}
	}

	return ids, err
}

func (col *Collection) getGatewayName(networkID *string) (string, error) {
	nw, err := col.getNetwork(networkID)
	if err != nil {
		return "", err
	}

	tags := mapEC2Tags(nw.Tags)
	name := tags["ernest.nat_gateway"]

	return name, nil
}

func (col *Collection) getNetwork(id *string) (*ec2.Subnet, error) {
	svc := col.getEC2Client()

	req := &ec2.DescribeSubnetsInput{
		SubnetIds: []*string{id},
	}

	resp, err := svc.DescribeSubnets(req)
	if err != nil {
		return nil, err
	}

	if len(resp.Subnets) != 1 {
		return nil, errors.New("could not find referenced nat gateway subnet")
	}

	return resp.Subnets[0], nil
}

func mapEC2Tags(input []*ec2.Tag) map[string]string {
	t := make(map[string]string)

	for _, tag := range input {
		t[*tag.Key] = *tag.Value
	}

	return t
}

// ToEvent converts an ec2 nat gateway object to an ernest event
func toEvent(ng *ec2.NatGateway, name string) *Event {
	id, ip := getPublicAllocation(ng.NatGatewayAddresses)

	e := &Event{
		ProviderType:           "aws",
		ComponentType:          "nat",
		ComponentID:            "nat::" + name,
		Name:                   aws.String(name),
		VpcID:                  *ng.VpcId,
		NatGatewayAWSID:        ng.NatGatewayId,
		PublicNetworkAWSID:     ng.SubnetId,
		NatGatewayAllocationID: id,
		NatGatewayAllocationIP: ip,
		//RoutedNetworksAWSIDs
		//InternetGatewayID
	}
	return e
}
