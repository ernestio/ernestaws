/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package s3

import (
	"encoding/json"
	"errors"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ernestio/ernestaws/credentials"
)

// Collection ....
type Collection struct {
	UUID               string            `json:"_uuid"`
	BatchID            string            `json:"_batch_id"`
	ProviderType       string            `json:"_type"`
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

	col.Body, err = json.Marshal(col)
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

// Find : Find s3 buckets on aws
func (col *Collection) Find() error {
	svc := col.getS3Client()

	resp, err := svc.ListBuckets(nil)
	if err != nil {
		return err
	}

	for _, b := range resp.Buckets {
		tags, _ := getBucketTagDescriptions(svc, b.Name)

		grants, err := getBucketPermissions(svc, b.Name)
		if err != nil {
			return err
		}

		location, err := getBucketLocation(svc, b.Name)
		if err != nil {
			return err
		}

		event := toEvent(b, grants, location, tags)

		if tagsMatch(col.Tags, event.Tags) {
			col.Results = append(col.Results, event)
		}
	}

	return nil
}

func (col *Collection) getS3Client() *s3.S3 {
	creds, _ := credentials.NewStaticCredentials(col.AWSAccessKeyID, col.AWSSecretAccessKey, col.CryptoKey)
	return s3.New(session.New(), &aws.Config{
		Region:      aws.String(col.DatacenterRegion),
		Credentials: creds,
	})
}

func mapS3Tags(input []*s3.Tag) map[string]string {
	t := make(map[string]string)

	for _, tag := range input {
		t[*tag.Key] = *tag.Value
	}

	return t
}

func tagsMatch(qt, rt map[string]string) bool {
	for k, v := range qt {
		if rt[k] != v {
			return false
		}
	}

	return true
}

func getBucketTagDescriptions(svc *s3.S3, name *string) ([]*s3.Tag, error) {
	treq := &s3.GetBucketTaggingInput{
		Bucket: name,
	}

	resp, err := svc.GetBucketTagging(treq)

	return resp.TagSet, err
}

func getBucketPermissions(svc *s3.S3, name *string) ([]*s3.Grant, error) {
	req := &s3.GetBucketAclInput{
		Bucket: name,
	}

	resp, err := svc.GetBucketAcl(req)

	return resp.Grants, err
}

func getBucketLocation(svc *s3.S3, name *string) (*string, error) {
	req := &s3.GetBucketLocationInput{
		Bucket: name,
	}

	resp, err := svc.GetBucketLocation(req)

	return resp.LocationConstraint, err
}

func mapS3Grantees(grantees []*s3.Grant) []Grantee {
	var gs []Grantee

	for _, g := range grantees {
		grantee := Grantee{
			ID:          g.Grantee.ID,
			Type:        g.Grantee.Type,
			Permissions: g.Permission,
		}

		switch *g.Grantee.Type {
		case "id":
			grantee.ID = g.Grantee.ID
		case "emailaddress":
			grantee.ID = g.Grantee.EmailAddress
		case "uri":
			grantee.ID = g.Grantee.URI
		}

		gs = append(gs, grantee)
	}

	return gs
}

// ToEvent converts an s3 bucket object to an ernest event
func toEvent(b *s3.Bucket, grants []*s3.Grant, location *string, tags []*s3.Tag) *Event {
	e := &Event{
		Name:           b.Name,
		Grantees:       mapS3Grantees(grants),
		BucketLocation: location,
		// ACL ???
		// BucketURI
		Tags: mapS3Tags(tags),
	}
	return e
}
