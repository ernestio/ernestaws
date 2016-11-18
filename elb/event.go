/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package elb

import (
	"encoding/json"
	"errors"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/ernestio/ernestaws"
)

var (
	// ErrDatacenterIDInvalid ...
	ErrDatacenterIDInvalid = errors.New("Datacenter VPC ID invalid")
	// ErrDatacenterRegionInvalid ...
	ErrDatacenterRegionInvalid = errors.New("Datacenter Region invalid")
	// ErrDatacenterCredentialsInvalid ...
	ErrDatacenterCredentialsInvalid = errors.New("Datacenter credentials invalid")
	// ErrELBNameInvalid ...
	ErrELBNameInvalid = errors.New("ELB name is invalid")
	// ErrELBProtocolInvalid ...
	ErrELBProtocolInvalid = errors.New("ELB protocol invalid")
	// ErrELBFromPortInvalid ...
	ErrELBFromPortInvalid = errors.New("ELB from port invalid")
	// ErrELBToPortInvalid ...
	ErrELBToPortInvalid = errors.New("ELB to port invalid")
)

// Listener ...
type Listener struct {
	FromPort  int64  `json:"from_port"`
	ToPort    int64  `json:"to_port"`
	Protocol  string `json:"protocol"`
	SSLCertID string `json:"ssl_cert"`
}

// Event stores the template data
type Event struct {
	UUID                string     `json:"_uuid"`
	BatchID             string     `json:"_batch_id"`
	ProviderType        string     `json:"_type"`
	DatacenterName      string     `json:"datacenter_name,omitempty"`
	DatacenterRegion    string     `json:"datacenter_region"`
	DatacenterToken     string     `json:"datacenter_token"`
	DatacenterSecret    string     `json:"datacenter_secret"`
	VPCID               string     `json:"vpc_id"`
	ELBName             string     `json:"name"`
	ELBIsPrivate        bool       `json:"is_private"`
	ELBListeners        []Listener `json:"listeners"`
	ELBDNSName          string     `json:"dns_name"`
	InstanceNames       []string   `json:"instance_names"`
	InstanceAWSIDs      []string   `json:"instance_aws_ids"`
	NetworkAWSIDs       []string   `json:"network_aws_ids"`
	SecurityGroups      []string   `json:"security_groups"`
	SecurityGroupAWSIDs []string   `json:"security_group_aws_ids"`
	ErrorMessage        string     `json:"error,omitempty"`
	Subject             string     `json:"-"`
	Body                []byte     `json:"-"`
}

// New : Constructor
func New(subject string, body []byte) ernestaws.Event {
	n := Event{Subject: subject, Body: body}

	return &n
}

// GetBody : Gets the body for this event
func (ev *Event) GetBody() []byte {
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

	if ev.DatacenterSecret == "" || ev.DatacenterToken == "" {
		return ErrDatacenterCredentialsInvalid
	}

	if ev.ELBName == "" {
		return ErrELBNameInvalid
	}

	if ev.Subject != "elb.delete.aws" {
		// Validate Ports
		for _, listener := range ev.ELBListeners {
			if listener.Protocol == "" {
				return ErrELBProtocolInvalid
			}
			if listener.FromPort < 1 || listener.FromPort > 65535 {
				return ErrELBFromPortInvalid
			}
			if listener.ToPort < 1 || listener.ToPort > 65535 {
				return ErrELBToPortInvalid
			}

			if listener.Protocol != "HTTP" &&
				listener.Protocol != "HTTPS" &&
				listener.Protocol != "TCP" &&
				listener.Protocol != "SSL" {
				return ErrELBProtocolInvalid
			}
		}
	}

	return nil
}

// Create : Creates a elb object on aws
func (ev *Event) Create() error {
	creds := credentials.NewStaticCredentials(ev.DatacenterSecret, ev.DatacenterToken, "")
	svc := elb.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})

	// Create Loadbalancer
	req := elb.CreateLoadBalancerInput{
		LoadBalancerName: aws.String(ev.ELBName),
		Listeners:        ev.mapListeners(),
	}

	if ev.ELBIsPrivate {
		req.Scheme = aws.String("internal")
	}

	for _, sg := range ev.SecurityGroupAWSIDs {
		req.SecurityGroups = append(req.SecurityGroups, aws.String(sg))
	}

	for _, subnet := range ev.NetworkAWSIDs {
		req.Subnets = append(req.Subnets, aws.String(subnet))
	}

	resp, err := svc.CreateLoadBalancer(&req)
	if err != nil {
		return err
	}

	if resp.DNSName != nil {
		ev.ELBDNSName = *resp.DNSName
	}

	// Add instances
	ireq := elb.RegisterInstancesWithLoadBalancerInput{
		LoadBalancerName: aws.String(ev.ELBName),
	}

	for _, instance := range ev.InstanceAWSIDs {
		ireq.Instances = append(ireq.Instances, &elb.Instance{
			InstanceId: aws.String(instance),
		})
	}

	_, err = svc.RegisterInstancesWithLoadBalancer(&ireq)
	if err != nil {
		return err
	}

	return nil
}

// Update : Updates a elb object on aws
func (ev *Event) Update() error {
	creds := credentials.NewStaticCredentials(ev.DatacenterSecret, ev.DatacenterToken, "")
	svc := elb.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})

	req := elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{aws.String(ev.ELBName)},
	}

	resp, err := svc.DescribeLoadBalancers(&req)
	if err != nil {
		return err
	}

	if len(resp.LoadBalancerDescriptions) != 1 {
		return errors.New("Could not find ELB")
	}

	lb := resp.LoadBalancerDescriptions[0]

	// Update ports, certs and security groups & networks
	err = ev.updateELBSecurityGroups(svc, lb, ev.SecurityGroupAWSIDs)
	if err != nil {
		return err
	}

	err = ev.updateELBNetworks(svc, lb, ev.NetworkAWSIDs)
	if err != nil {
		return err
	}

	err = ev.updateELBInstances(svc, lb, ev.InstanceAWSIDs)
	if err != nil {
		return err
	}

	err = ev.updateELBListeners(svc, lb, ev.ELBListeners)
	if err != nil {
		return err
	}

	return nil
}

// Delete : Deletes a elb object on aws
func (ev *Event) Delete() error {
	creds := credentials.NewStaticCredentials(ev.DatacenterSecret, ev.DatacenterToken, "")
	svc := elb.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})

	// Delete Loadbalancer
	req := elb.DeleteLoadBalancerInput{
		LoadBalancerName: aws.String(ev.ELBName),
	}

	_, err := svc.DeleteLoadBalancer(&req)
	if err != nil {
		return err
	}

	return nil
}

// Get : Gets a elb object on aws
func (ev *Event) Get() error {
	return errors.New(ev.Subject + " not supported")
}

func (ev *Event) mapListeners() []*elb.Listener {
	var l []*elb.Listener

	for _, port := range ev.ELBListeners {
		l = append(l, &elb.Listener{
			Protocol:         aws.String(port.Protocol),
			LoadBalancerPort: aws.Int64(port.FromPort),
			InstancePort:     aws.Int64(port.ToPort),
			InstanceProtocol: aws.String(port.Protocol),
			SSLCertificateId: aws.String(port.SSLCertID),
		})
	}

	return l
}

func (ev *Event) updateELBInstances(svc *elb.ELB, lb *elb.LoadBalancerDescription, ni []string) error {
	var err error

	// Instances to remove
	drreq := elb.DeregisterInstancesFromLoadBalancerInput{
		LoadBalancerName: lb.LoadBalancerName,
		Instances:        ev.instancesToDeregister(ni, lb.Instances),
	}
	if len(drreq.Instances) > 0 {
		_, err = svc.DeregisterInstancesFromLoadBalancer(&drreq)
		if err != nil {
			return err
		}
	}

	// Instances to add
	rreq := elb.RegisterInstancesWithLoadBalancerInput{
		LoadBalancerName: lb.LoadBalancerName,
		Instances:        ev.instancesToRegister(ni, lb.Instances),
	}

	if len(rreq.Instances) > 0 {
		_, err = svc.RegisterInstancesWithLoadBalancer(&rreq)
	}

	return err
}

func (ev *Event) updateELBListeners(svc *elb.ELB, lb *elb.LoadBalancerDescription, nl []Listener) error {
	var err error

	dlreq := elb.DeleteLoadBalancerListenersInput{
		LoadBalancerName:  lb.LoadBalancerName,
		LoadBalancerPorts: ev.listenersToDelete(nl, lb.ListenerDescriptions),
	}

	if len(dlreq.LoadBalancerPorts) > 0 {
		_, err = svc.DeleteLoadBalancerListeners(&dlreq)
		if err != nil {
			return err
		}
	}

	clreq := elb.CreateLoadBalancerListenersInput{
		LoadBalancerName: lb.LoadBalancerName,
		Listeners:        ev.listenersToCreate(nl, lb.ListenerDescriptions),
	}

	if len(clreq.Listeners) > 0 {
		_, err = svc.CreateLoadBalancerListeners(&clreq)
	}

	return err
}

func (ev *Event) updateELBNetworks(svc *elb.ELB, lb *elb.LoadBalancerDescription, nl []string) error {
	var err error

	dsreq := elb.DetachLoadBalancerFromSubnetsInput{
		LoadBalancerName: lb.LoadBalancerName,
		Subnets:          ev.subnetsToDetach(nl, lb.Subnets),
	}

	if len(dsreq.Subnets) > 0 {
		_, err = svc.DetachLoadBalancerFromSubnets(&dsreq)
		if err != nil {
			return err
		}
	}

	csreq := elb.AttachLoadBalancerToSubnetsInput{
		LoadBalancerName: lb.LoadBalancerName,
		Subnets:          ev.subnetsToAttach(nl, lb.Subnets),
	}

	if len(csreq.Subnets) > 0 {
		_, err = svc.AttachLoadBalancerToSubnets(&csreq)
	}

	return err
}

func (ev *Event) updateELBSecurityGroups(svc *elb.ELB, lb *elb.LoadBalancerDescription, nsg []string) error {
	var err error
	var sgs []*string

	for _, sg := range nsg {
		sgs = append(sgs, aws.String(sg))
	}

	req := elb.ApplySecurityGroupsToLoadBalancerInput{
		LoadBalancerName: lb.LoadBalancerName,
		SecurityGroups:   sgs,
	}

	if len(req.SecurityGroups) > 0 {
		_, err = svc.ApplySecurityGroupsToLoadBalancer(&req)
	}

	return err
}

func (ev *Event) portInUse(listeners []*elb.ListenerDescription, port int64) bool {
	for _, l := range listeners {
		if *l.Listener.LoadBalancerPort == port {
			return true
		}
	}

	return false
}

func (ev *Event) portRemoved(ports []Listener, listener *elb.ListenerDescription) bool {
	for _, p := range ports {
		if p.FromPort == *listener.Listener.LoadBalancerPort {
			return false
		}
	}

	return true
}

func (ev *Event) instancesToRegister(newInstances []string, currentInstances []*elb.Instance) []*elb.Instance {
	var i []*elb.Instance

	for _, instance := range newInstances {
		exists := false
		for _, ci := range currentInstances {
			if instance == *ci.InstanceId {
				exists = true
			}
		}
		if exists != true {
			i = append(i, &elb.Instance{InstanceId: aws.String(instance)})
		}
	}

	return i
}

func (ev *Event) instancesToDeregister(newInstances []string, currentInstances []*elb.Instance) []*elb.Instance {
	var i []*elb.Instance

	for _, ci := range currentInstances {
		exists := false
		for _, instance := range newInstances {
			if *ci.InstanceId == instance {
				exists = true
			}
		}
		if exists != true {
			i = append(i, &elb.Instance{InstanceId: ci.InstanceId})
		}
	}

	return i
}

func (ev *Event) listenersToDelete(newListeners []Listener, currentListeners []*elb.ListenerDescription) []*int64 {
	var l []*int64

	for _, cl := range currentListeners {
		if ev.portRemoved(newListeners, cl) {
			l = append(l, cl.Listener.LoadBalancerPort)
		}
	}

	return l
}

func (ev *Event) listenersToCreate(newListeners []Listener, currentListeners []*elb.ListenerDescription) []*elb.Listener {
	var l []*elb.Listener

	for _, listener := range newListeners {

		if ev.portInUse(currentListeners, listener.FromPort) != true {
			l = append(l, &elb.Listener{
				Protocol:         aws.String(listener.Protocol),
				LoadBalancerPort: aws.Int64(listener.FromPort),
				InstancePort:     aws.Int64(listener.ToPort),
				InstanceProtocol: aws.String(listener.Protocol),
				SSLCertificateId: aws.String(listener.SSLCertID),
			})
		}
	}

	return l
}

func (ev *Event) subnetsToAttach(newSubnets []string, currentSubnets []*string) []*string {
	var s []*string

	for _, subnet := range newSubnets {
		exists := false
		for _, cs := range currentSubnets {
			if subnet == *cs {
				exists = true
			}
		}
		if exists != true {
			s = append(s, aws.String(subnet))
		}
	}

	return s
}

func (ev *Event) subnetsToDetach(newSubnets []string, currentSubnets []*string) []*string {
	var s []*string

	for _, cs := range currentSubnets {
		exists := false
		for _, subnet := range newSubnets {
			if *cs == subnet {
				exists = true
			}
		}
		if exists != true {
			s = append(s, cs)
		}
	}

	return s
}
