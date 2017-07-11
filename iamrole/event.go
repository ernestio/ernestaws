/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package iamrole

import (
	"encoding/json"
	"errors"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
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
	ProviderType         string    `json:"_provider"`
	ComponentType        string    `json:"_component"`
	ComponentID          string    `json:"_component_id"`
	State                string    `json:"_state"`
	Action               string    `json:"_action"`
	IAMRoleAWSID         *string   `json:"iam_role_aws_id"`
	IAMRoleARN           *string   `json:"iam_role_arn"`
	Name                 *string   `json:"name"`
	AssumePolicyDocument *string   `json:"assume_policy_document"`
	Policies             []string  `json:"policies"`
	PolicyARNs           []*string `json:"policy_arns"`
	Description          *string   `json:"description"`
	Path                 *string   `json:"path"`
	DatacenterRegion     string    `json:"datacenter_region"`
	AccessKeyID          string    `json:"aws_access_key_id"`
	SecretAccessKey      string    `json:"aws_secret_access_key"`
	Service              string    `json:"service"`
	ErrorMessage         string    `json:"error,omitempty"`
	Subject              string    `json:"-"`
	Body                 []byte    `json:"-"`
	CryptoKey            string    `json:"-"`
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
	if ev.DatacenterRegion == "" {
		return ErrDatacenterRegionInvalid
	}

	if ev.AccessKeyID == "" || ev.SecretAccessKey == "" {
		return ErrDatacenterCredentialsInvalid
	}

	if ev.Subject == "iam_role.delete.aws" {
		if ev.IAMRoleAWSID == nil {
			return ErrNetworkAWSIDInvalid
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

// Create : Creates a role object on aws
func (ev *Event) Create() error {
	svc := ev.getIAMClient()

	req := &iam.CreateRoleInput{
		RoleName:                 ev.Name,
		Description:              ev.Description,
		AssumeRolePolicyDocument: ev.AssumePolicyDocument,
		Path: ev.Path,
	}

	resp, err := svc.CreateRole(req)
	if err != nil {
		return err
	}

	ev.IAMRoleAWSID = resp.Role.RoleId
	ev.IAMRoleARN = resp.Role.Arn

	for _, arn := range ev.PolicyARNs {
		areq := &iam.AttachRolePolicyInput{
			RoleName:  ev.Name,
			PolicyArn: arn,
		}

		_, err := svc.AttachRolePolicy(areq)
		if err != nil {
			return err
		}
	}

	return nil
}

// Update : Updates a role object on aws
func (ev *Event) Update() error {
	return errors.New(ev.Subject + " not supported")
}

// Delete : Deletes a role object on aws
func (ev *Event) Delete() error {
	svc := ev.getIAMClient()

	for _, arn := range ev.PolicyARNs {
		dreq := &iam.DetachRolePolicyInput{
			RoleName:  ev.Name,
			PolicyArn: arn,
		}

		_, err := svc.DetachRolePolicy(dreq)
		if err != nil {
			return err
		}
	}

	req := &iam.DeleteRoleInput{
		RoleName: ev.Name,
	}

	_, err := svc.DeleteRole(req)

	return err
}

// Get : Gets a role object on aws
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

func (ev *Event) getIAMClient() *iam.IAM {
	creds, _ := credentials.NewStaticCredentials(ev.AccessKeyID, ev.SecretAccessKey, ev.CryptoKey)
	return iam.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})
}
