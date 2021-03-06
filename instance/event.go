/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package instance

import (
	"encoding/base64"
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
	// ErrInstanceAWSIDInvalid ...
	ErrInstanceAWSIDInvalid = errors.New("Instance aws id invalid")
	// ErrNetworkInvalid ...
	ErrNetworkInvalid = errors.New("Network invalid")
	// ErrInstanceNameInvalid ...
	ErrInstanceNameInvalid = errors.New("Instance name invalid")
	// ErrInstanceImageInvalid ...
	ErrInstanceImageInvalid = errors.New("Instance image invalid")
	// ErrInstanceTypeInvalid ...
	ErrInstanceTypeInvalid = errors.New("Instance type invalid")
)

// Volume stores ebs volume data
type Volume struct {
	Volume      *string `json:"volume"`
	Device      *string `json:"device"`
	VolumeAWSID *string `json:"volume_aws_id"`
}

// Event stores the template data
type Event struct {
	ProviderType          string            `json:"_provider"`
	ComponentType         string            `json:"_component"`
	ComponentID           string            `json:"_component_id"`
	State                 string            `json:"_state"`
	Action                string            `json:"_action"`
	InstanceAWSID         *string           `json:"instance_aws_id"`
	Name                  *string           `json:"name"`
	Type                  *string           `json:"instance_type"`
	Image                 *string           `json:"image"`
	IP                    *string           `json:"ip"`
	PublicIP              *string           `json:"public_ip"`
	ElasticIP             *string           `json:"elastic_ip"`
	ElasticIPAWSID        *string           `json:"elastic_ip_aws_id,omitempty"`
	AssignElasticIP       *bool             `json:"assign_elastic_ip"`
	KeyPair               *string           `json:"key_pair"`
	UserData              *string           `json:"user_data"`
	Network               *string           `json:"network_name"`
	NetworkAWSID          *string           `json:"network_aws_id"`
	NetworkIsPublic       *bool             `json:"network_is_public"`
	SecurityGroups        []string          `json:"security_groups"`
	SecurityGroupAWSIDs   []*string         `json:"security_group_aws_ids"`
	IAMInstanceProfile    *string           `json:"iam_instance_profile"`
	IAMInstanceProfileARN *string           `json:"iam_instance_profile_arn"`
	Volumes               []Volume          `json:"volumes"`
	Tags                  map[string]string `json:"tags"`
	DatacenterType        string            `json:"datacenter_type,omitempty"`
	DatacenterName        string            `json:"datacenter_name,omitempty"`
	DatacenterRegion      string            `json:"datacenter_region"`
	AccessKeyID           string            `json:"aws_access_key_id"`
	SecretAccessKey       string            `json:"aws_secret_access_key"`
	Service               string            `json:"service"`
	Powered               bool              `json:"powered"`
	ErrorMessage          string            `json:"error,omitempty"`
	Subject               string            `json:"-"`
	Body                  []byte            `json:"-"`
	CryptoKey             string            `json:"-"`
}

// New : Constructor
func New(subject string, body []byte, cryptoKey string) ernestaws.Event {
	if strings.Split(subject, ".")[1] == "find" {
		return &Collection{Subject: subject, Body: body, CryptoKey: cryptoKey}
	}

	return &Event{Subject: subject, Body: body, CryptoKey: cryptoKey, Powered: true}
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

	if ev.Subject != "instance.create.aws" {
		if ev.InstanceAWSID == nil {
			return ErrInstanceAWSIDInvalid
		}
	}

	if ev.Subject != "instance.delete.aws" {
		if ev.NetworkAWSID == nil {
			return ErrNetworkInvalid
		}
	}

	if ev.Name == nil {
		return ErrInstanceNameInvalid
	}

	if ev.Image == nil {
		return ErrInstanceImageInvalid
	}

	if ev.Type == nil {
		return ErrInstanceTypeInvalid
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

	req := ec2.RunInstancesInput{
		SubnetId:         ev.NetworkAWSID,
		ImageId:          ev.Image,
		InstanceType:     ev.Type,
		PrivateIpAddress: ev.IP,
		KeyName:          ev.KeyPair,
		MaxCount:         aws.Int64(1),
		MinCount:         aws.Int64(1),
	}

	for _, sg := range ev.SecurityGroupAWSIDs {
		req.SecurityGroupIds = append(req.SecurityGroupIds, sg)
	}

	if ev.UserData != nil {
		req.UserData = ev.encodeUserData(ev.UserData)
	}

	if ev.IAMInstanceProfile != nil {
		req.IamInstanceProfile = &ec2.IamInstanceProfileSpecification{
			Name: ev.IAMInstanceProfile,
		}
	}

	resp, err := svc.RunInstances(&req)
	if err != nil {
		return err
	}

	builtInstance := ec2.DescribeInstancesInput{
		InstanceIds: []*string{resp.Instances[0].InstanceId},
	}

	err = svc.WaitUntilInstanceRunning(&builtInstance)
	if err != nil {
		return err
	}

	if *ev.AssignElasticIP {
		ev.ElasticIP, ev.ElasticIPAWSID, err = ev.assignElasticIP(svc, resp.Instances[0].InstanceId)
		if err != nil {
			return err
		}
	}

	ev.InstanceAWSID = resp.Instances[0].InstanceId

	instance, err := ev.getInstanceByID(resp.Instances[0].InstanceId)
	if err != nil {
		return err
	}

	ev.PublicIP = instance.PublicIpAddress

	err = ev.setTags()
	if err != nil {
		return err
	}

	return ev.attachVolumes()
}

// Update : Updates a instance object on aws
func (ev *Event) Update() error {
	var err error
	svc := ev.getEC2Client()

	builtInstance := ec2.DescribeInstancesInput{
		InstanceIds: []*string{ev.InstanceAWSID},
	}

	okInstance := ec2.DescribeInstanceStatusInput{
		InstanceIds: []*string{ev.InstanceAWSID},
	}

	input := ec2.DescribeInstanceStatusInput{
		InstanceIds:         append([]*string{}, ev.InstanceAWSID),
		IncludeAllInstances: aws.Bool(true),
	}
	output, _ := svc.DescribeInstanceStatus(&input)
	status := output.InstanceStatuses[0].InstanceState.Code

	if *status != 80 {
		err := svc.WaitUntilInstanceStatusOk(&okInstance)
		if err != nil {
			log.Println("[ERROR]: Waiting for instance to be in status OK")
			return err
		}

		stopreq := ec2.StopInstancesInput{
			InstanceIds: []*string{ev.InstanceAWSID},
		}

		// power off the instance
		_, err = svc.StopInstances(&stopreq)
		if err != nil {
			log.Println("[ERROR]: While stopping the instance")
			return err
		}

		err = svc.WaitUntilInstanceStopped(&builtInstance)
		if err != nil {
			log.Println("[ERROR]: Waiting until instance is stopped")
			return err
		}
	}

	// resize the instance
	req := ec2.ModifyInstanceAttributeInput{
		InstanceId: ev.InstanceAWSID,
		InstanceType: &ec2.AttributeValue{
			Value: ev.Type,
		},
	}

	_, err = svc.ModifyInstanceAttribute(&req)
	if err != nil {
		log.Println("[ERROR]: Modifying instance attributes (I)")
		return err
	}

	// update instance security groups
	req = ec2.ModifyInstanceAttributeInput{
		InstanceId: ev.InstanceAWSID,
		Groups:     []*string{},
	}

	for _, sg := range ev.SecurityGroupAWSIDs {
		req.Groups = append(req.Groups, sg)
	}

	_, err = svc.ModifyInstanceAttribute(&req)
	if err != nil {
		log.Println("[ERROR]: Modifying instance attributes (II)")
		return err
	}

	err = ev.attachVolumes()
	if err != nil {
		log.Println("[ERROR]: Attaching instance volumes")
		return err
	}

	if ev.Powered == true {
		// power the instance back on
		startreq := ec2.StartInstancesInput{
			InstanceIds: []*string{ev.InstanceAWSID},
		}

		_, err = svc.StartInstances(&startreq)
		if err != nil {
			log.Println("[ERROR] While starting the instance")
			return err
		}

		err = svc.WaitUntilInstanceRunning(&builtInstance)
		if err != nil {
			log.Println("[ERROR] While waiting for instance to be running")
			return err
		}

		instance, err := ev.getInstanceByID(ev.InstanceAWSID)
		if err != nil {
			log.Println("[ERROR]: Getting instance by id")
			return err
		}

		ev.PublicIP = instance.PublicIpAddress
	}

	return ev.setTags()
}

// Delete : Deletes a instance object on aws
func (ev *Event) Delete() error {
	svc := ev.getEC2Client()

	req := ec2.TerminateInstancesInput{
		InstanceIds: []*string{ev.InstanceAWSID},
	}

	_, err := svc.TerminateInstances(&req)
	if err != nil {
		return err
	}

	termreq := ec2.DescribeInstancesInput{
		InstanceIds: []*string{ev.InstanceAWSID},
	}

	err = svc.WaitUntilInstanceTerminated(&termreq)
	if err != nil {
		return err
	}

	if ev.ElasticIPAWSID != nil {
		rreq := &ec2.ReleaseAddressInput{
			AllocationId: ev.ElasticIPAWSID,
		}

		_, err = svc.ReleaseAddress(rreq)
	}

	return err
}

// Get : Gets a instance object on aws
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

func (ev *Event) assignElasticIP(svc *ec2.EC2, instanceID *string) (*string, *string, error) {
	// Create Elastic IP
	resp, err := svc.AllocateAddress(nil)
	if err != nil {
		return nil, nil, err
	}

	req := ec2.AssociateAddressInput{
		InstanceId:   instanceID,
		AllocationId: resp.AllocationId,
	}
	_, err = svc.AssociateAddress(&req)
	if err != nil {
		return nil, nil, err
	}

	return resp.PublicIp, resp.AllocationId, nil
}

func (ev *Event) getInstanceByID(id *string) (*ec2.Instance, error) {
	svc := ev.getEC2Client()

	req := ec2.DescribeInstancesInput{
		InstanceIds: []*string{id},
	}

	resp, err := svc.DescribeInstances(&req)
	if err != nil {
		return nil, err
	}

	if len(resp.Reservations) != 1 {
		return nil, errors.New("Could not find any instance reservations")
	}

	if len(resp.Reservations[0].Instances) != 1 {
		return nil, errors.New("Could not find an instance with that ID")
	}

	return resp.Reservations[0].Instances[0], nil
}

func (ev *Event) encodeUserData(data *string) *string {
	value := base64.StdEncoding.EncodeToString([]byte(*data))
	return &value
}

func (ev *Event) attachVolumes() error {
	svc := ev.getEC2Client()

	instance, err := ev.getInstanceByID(ev.InstanceAWSID)
	if err != nil {
		return err
	}

	for _, bdm := range instance.BlockDeviceMappings {
		if hasBlockDevice(ev.Volumes, bdm) || *bdm.DeviceName == *instance.RootDeviceName {
			continue
		}

		req := &ec2.DetachVolumeInput{
			InstanceId: ev.InstanceAWSID,
			VolumeId:   bdm.Ebs.VolumeId,
		}

		_, err = svc.DetachVolume(req)
		if err != nil {
			return err
		}
	}

	for _, vol := range ev.Volumes {
		// check volume doesn't exist
		if hasVolumeAttached(instance.BlockDeviceMappings, vol) {
			continue
		}

		req := &ec2.AttachVolumeInput{
			Device:     vol.Device,
			VolumeId:   vol.VolumeAWSID,
			InstanceId: ev.InstanceAWSID,
		}

		_, err = svc.AttachVolume(req)
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
			Resources: []*string{ev.InstanceAWSID},
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

func hasVolumeAttached(bdms []*ec2.InstanceBlockDeviceMapping, vol Volume) bool {
	for _, bdm := range bdms {
		if *bdm.Ebs.VolumeId == *vol.VolumeAWSID || *bdm.DeviceName == *vol.Device {
			return true
		}
	}

	return false
}

func hasBlockDevice(volumes []Volume, bdm *ec2.InstanceBlockDeviceMapping) bool {
	for _, vol := range volumes {
		if *vol.VolumeAWSID == *bdm.Ebs.VolumeId || *vol.Device == *bdm.DeviceName {
			return true
		}
	}

	return false
}

func mapEC2Tags(input []*ec2.Tag) map[string]string {
	t := make(map[string]string)

	for _, tag := range input {
		t[*tag.Key] = *tag.Value
	}

	return t
}
