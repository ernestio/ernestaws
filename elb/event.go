/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package elb

import (
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
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
	FromPort  *int64  `json:"from_port"`
	ToPort    *int64  `json:"to_port"`
	Protocol  *string `json:"protocol"`
	SSLCertID *string `json:"ssl_cert"`
}

// Event stores the template data
type Event struct {
	ProviderType        string            `json:"_provider"`
	ComponentType       string            `json:"_component"`
	ComponentID         string            `json:"_component_id"`
	State               string            `json:"_state"`
	Action              string            `json:"_action"`
	Name                *string           `json:"name"`
	IsPrivate           *bool             `json:"is_private"`
	Listeners           []Listener        `json:"listeners"`
	DNSName             *string           `json:"dns_name"`
	Instances           []string          `json:"instances"`
	InstanceNames       []string          `json:"instance_names"`
	InstanceAWSIDs      []*string         `json:"instance_aws_ids"`
	Networks            []string          `json:"networks"`
	NetworkAWSIDs       []*string         `json:"network_aws_ids"`
	SecurityGroups      []string          `json:"security_groups"`
	SecurityGroupAWSIDs []*string         `json:"security_group_aws_ids"`
	Tags                map[string]string `json:"tags"`
	DatacenterType      string            `json:"datacenter_type,omitempty"`
	DatacenterName      string            `json:"datacenter_name,omitempty"`
	DatacenterRegion    string            `json:"datacenter_region"`
	AccessKeyID         string            `json:"aws_access_key_id"`
	SecretAccessKey     string            `json:"aws_secret_access_key"`
	Service             string            `json:"service"`
	ErrorMessage        string            `json:"error,omitempty"`
	Subject             string            `json:"-"`
	Body                []byte            `json:"-"`
	CryptoKey           string            `json:"-"`
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

	if ev.Name == nil {
		return ErrELBNameInvalid
	}

	if ev.Subject != "elb.delete.aws" {
		// Validate Ports
		for _, listener := range ev.Listeners {
			if listener.Protocol == nil {
				return ErrELBProtocolInvalid
			}
			if *listener.FromPort < 1 || *listener.FromPort > 65535 {
				return ErrELBFromPortInvalid
			}
			if *listener.ToPort < 1 || *listener.ToPort > 65535 {
				return ErrELBToPortInvalid
			}

			if *listener.Protocol != "HTTP" &&
				*listener.Protocol != "HTTPS" &&
				*listener.Protocol != "TCP" &&
				*listener.Protocol != "SSL" {
				return ErrELBProtocolInvalid
			}
		}
	}

	return nil
}

// Find : Find an object on aws
func (ev *Event) Find() error {
	return errors.New(ev.Subject + " not supported")
}

// Create : Creates a elb object on aws
func (ev *Event) Create() error {
	svc := ev.getELBClient()

	// Create Loadbalancer
	req := elb.CreateLoadBalancerInput{
		LoadBalancerName: ev.Name,
		Listeners:        ev.mapListeners(),
		Subnets:          ev.NetworkAWSIDs,
		SecurityGroups:   ev.SecurityGroupAWSIDs,
	}

	if ev.IsPrivate != nil {
		if *ev.IsPrivate {
			req.Scheme = aws.String("internal")
		}
	}

	resp, err := svc.CreateLoadBalancer(&req)
	if err != nil {
		return err
	}

	ev.DNSName = resp.DNSName

	// Add instances
	ireq := elb.RegisterInstancesWithLoadBalancerInput{
		LoadBalancerName: ev.Name,
	}

	for _, instance := range ev.InstanceAWSIDs {
		ireq.Instances = append(ireq.Instances, &elb.Instance{
			InstanceId: instance,
		})
	}

	_, err = svc.RegisterInstancesWithLoadBalancer(&ireq)
	if err != nil {
		return err
	}

	return ev.setTags()
}

// Update : Updates a elb object on aws
func (ev *Event) Update() error {
	svc := ev.getELBClient()

	req := elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{ev.Name},
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

	err = ev.updateELBListeners(svc, lb, ev.Listeners)
	if err != nil {
		return err
	}

	return ev.setTags()
}

// Delete : Deletes a elb object on aws
func (ev *Event) Delete() error {
	svc := ev.getELBClient()

	// Delete Loadbalancer
	req := elb.DeleteLoadBalancerInput{
		LoadBalancerName: ev.Name,
	}

	_, err := svc.DeleteLoadBalancer(&req)
	if err != nil {
		return err
	}

	return ev.waitForELBRemoval(ev.Name)
}

// Get : Gets a elb object on aws
func (ev *Event) Get() error {
	return errors.New(ev.Subject + " not supported")
}

func (ev *Event) mapListeners() []*elb.Listener {
	var l []*elb.Listener

	for _, port := range ev.Listeners {
		l = append(l, &elb.Listener{
			Protocol:         port.Protocol,
			LoadBalancerPort: port.FromPort,
			InstancePort:     port.ToPort,
			InstanceProtocol: port.Protocol,
			SSLCertificateId: port.SSLCertID,
		})
	}

	return l
}

func (ev *Event) getELBClient() *elb.ELB {
	creds, _ := credentials.NewStaticCredentials(ev.AccessKeyID, ev.SecretAccessKey, ev.CryptoKey)
	return elb.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})
}

func (ev *Event) updateELBInstances(svc *elb.ELB, lb *elb.LoadBalancerDescription, ni []*string) error {
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

func (ev *Event) updateELBNetworks(svc *elb.ELB, lb *elb.LoadBalancerDescription, nl []*string) error {
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

func (ev *Event) updateELBSecurityGroups(svc *elb.ELB, lb *elb.LoadBalancerDescription, nsg []*string) error {
	var err error

	req := elb.ApplySecurityGroupsToLoadBalancerInput{
		LoadBalancerName: lb.LoadBalancerName,
		SecurityGroups:   nsg,
	}

	if len(req.SecurityGroups) > 0 {
		_, err = svc.ApplySecurityGroupsToLoadBalancer(&req)
	}

	return err
}

func (ev *Event) portInUse(listeners []*elb.ListenerDescription, port *int64) bool {
	for _, l := range listeners {
		if *l.Listener.LoadBalancerPort == *port {
			return true
		}
	}

	return false
}

func (ev *Event) portRemoved(ports []Listener, listener *elb.ListenerDescription) bool {
	for _, p := range ports {
		if *p.FromPort == *listener.Listener.LoadBalancerPort {
			return false
		}
	}

	return true
}

func (ev *Event) instancesToRegister(newInstances []*string, currentInstances []*elb.Instance) []*elb.Instance {
	var i []*elb.Instance

	for _, instance := range newInstances {
		exists := false
		for _, ci := range currentInstances {
			if *instance == *ci.InstanceId {
				exists = true
			}
		}
		if exists != true {
			i = append(i, &elb.Instance{InstanceId: instance})
		}
	}

	return i
}

func (ev *Event) instancesToDeregister(newInstances []*string, currentInstances []*elb.Instance) []*elb.Instance {
	var i []*elb.Instance

	for _, ci := range currentInstances {
		exists := false
		for _, instance := range newInstances {
			if *ci.InstanceId == *instance {
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
				Protocol:         listener.Protocol,
				LoadBalancerPort: listener.FromPort,
				InstancePort:     listener.ToPort,
				InstanceProtocol: listener.Protocol,
				SSLCertificateId: listener.SSLCertID,
			})
		}
	}

	return l
}

func (ev *Event) subnetsToAttach(newSubnets []*string, currentSubnets []*string) []*string {
	var s []*string

	for _, subnet := range newSubnets {
		exists := false
		for _, cs := range currentSubnets {
			if *subnet == *cs {
				exists = true
			}
		}
		if exists != true {
			s = append(s, subnet)
		}
	}

	return s
}

func (ev *Event) subnetsToDetach(newSubnets []*string, currentSubnets []*string) []*string {
	var s []*string

	for _, cs := range currentSubnets {
		exists := false
		for _, subnet := range newSubnets {
			if *cs == *subnet {
				exists = true
			}
		}
		if exists != true {
			s = append(s, cs)
		}
	}

	return s
}

func (ev *Event) waitForELBRemoval(name *string) error {
	for {
		resp, err := ev.getELBs(name)
		if err != nil {
			return err
		}

		if len(resp.LoadBalancerDescriptions) == 0 {
			return nil
		}

		time.Sleep(time.Second)
	}
}

func (ev *Event) getELBs(name *string) (*elb.DescribeLoadBalancersOutput, error) {
	svc := ev.getELBClient()

	req := elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{name},
	}

	return svc.DescribeLoadBalancers(&req)
}

func (ev *Event) setTags() error {
	svc := ev.getELBClient()

	req := &elb.AddTagsInput{
		LoadBalancerNames: []*string{ev.Name},
	}

	for key, val := range ev.Tags {
		req.Tags = append(req.Tags, &elb.Tag{
			Key:   &key,
			Value: &val,
		})
	}

	_, err := svc.AddTags(req)

	return err
}
