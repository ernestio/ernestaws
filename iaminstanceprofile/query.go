/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package iaminstanceprofile

import (
	"encoding/json"
	"errors"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/ernestio/ernestaws/credentials"
)

// Collection ....
type Collection struct {
	ProviderType     string            `json:"_provider"`
	ComponentType    string            `json:"_component"`
	ComponentID      string            `json:"_component_id"`
	State            string            `json:"_state"`
	Action           string            `json:"_action"`
	Service          string            `json:"service"`
	AccessKeyID      string            `json:"aws_access_key_id"`
	SecretAccessKey  string            `json:"aws_secret_access_key"`
	DatacenterRegion string            `json:"datacenter_region"`
	Tags             map[string]string `json:"tags"`
	Results          []interface{}     `json:"components"`
	ErrorMessage     string            `json:"error,omitempty"`
	Subject          string            `json:"-"`
	Body             []byte            `json:"-"`
	CryptoKey        string            `json:"-"`
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
	if col.AccessKeyID == "" || col.SecretAccessKey == "" {
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

// Find : Find networks on aws
func (col *Collection) Find() error {
	svc := col.getIAMClient()

	resp, err := svc.ListInstanceProfiles(&iam.ListInstanceProfilesInput{})
	if err != nil {
		return err
	}

	for _, p := range resp.InstanceProfiles {
		col.Results = append(col.Results, toEvent(p))
	}

	return nil
}

func (col *Collection) getIAMClient() *iam.IAM {
	creds, _ := credentials.NewStaticCredentials(col.AccessKeyID, col.SecretAccessKey, col.CryptoKey)
	return iam.New(session.New(), &aws.Config{
		Region:      aws.String(col.DatacenterRegion),
		Credentials: creds,
	})
}

// ToEvent converts an ec2 subnet object to an ernest event
func toEvent(r *iam.InstanceProfile) *Event {
	var roles []*string
	for _, role := range r.Roles {
		roles = append(roles, role.RoleName)
	}

	return &Event{
		ProviderType:            "aws",
		ComponentType:           "iam_instance_profile",
		ComponentID:             "iam_instance_profile::" + *r.InstanceProfileName,
		IAMInstanceProfileAWSID: r.InstanceProfileId,
		IAMInstanceProfileARN:   r.Arn,
		Name:  r.InstanceProfileName,
		Path:  r.Path,
		Roles: roles,
	}
}
