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
	// ErrS3NameInvalid ...
	ErrS3NameInvalid = errors.New("S3 bucket name is invalid")
)

// Grantee ...
type Grantee struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Permissions string `json:"permissions"`
}

// Event stores the template data
type Event struct {
	UUID               string            `json:"_uuid"`
	BatchID            string            `json:"_batch_id"`
	ProviderType       string            `json:"_type"`
	DatacenterName     string            `json:"datacenter_name,omitempty"`
	DatacenterRegion   string            `json:"datacenter_region"`
	AWSAccessKeyID     string            `json:"aws_access_key_id"`
	AWSSecretAccessKey string            `json:"aws_secret_access_key"`
	Name               string            `json:"name"`
	ACL                string            `json:"acl"`
	BucketLocation     string            `json:"bucket_location"`
	BucketURI          string            `json:"bucket_uri"`
	Grantees           []Grantee         `json:"grantees"`
	Tags               map[string]string `json:"tags"`
	ErrorMessage       string            `json:"error,omitempty"`
	Subject            string            `json:"-"`
	Body               []byte            `json:"-"`
	CryptoKey          string            `json:"-"`
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
	if ev.DatacenterRegion == "" {
		return ErrDatacenterRegionInvalid
	}

	if ev.AWSAccessKeyID == "" || ev.AWSSecretAccessKey == "" {
		return ErrDatacenterCredentialsInvalid
	}

	if ev.Name == "" {
		return ErrS3NameInvalid
	}

	return nil
}

// Create : Creates a nat object on aws
func (ev *Event) Create() error {
	s3client := ev.getS3Client()

	params := &s3.CreateBucketInput{
		Bucket: aws.String(ev.Name),
		ACL:    aws.String(ev.ACL),
		CreateBucketConfiguration: &s3.CreateBucketConfiguration{
			LocationConstraint: aws.String(ev.BucketLocation),
		},
	}

	resp, err := s3client.CreateBucket(params)
	if err != nil {
		return err
	}

	req := s3.HeadBucketInput{
		Bucket: resp.Location,
	}

	err = s3client.WaitUntilBucketExists(&req)
	if err != nil {
		return err
	}

	ev.BucketURI = *resp.Location

	if len(ev.Grantees) < 1 {
		return nil
	}

	err = ev.Update()
	if err != nil {
		return err
	}

	return nil
}

// Update : Updates a nat object on aws
func (ev *Event) Update() error {
	s3client := ev.getS3Client()
	params := &s3.PutBucketAclInput{
		Bucket: aws.String(ev.Name),
		ACL:    aws.String(ev.ACL),
	}

	var grants []*s3.Grant

	for _, g := range ev.Grantees {
		var grantee s3.Grantee

		switch g.Type {
		case "id":
			grantee.Type = aws.String(s3.TypeCanonicalUser)
			grantee.ID = aws.String(g.ID)
		case "emailaddress":
			grantee.Type = aws.String(s3.TypeAmazonCustomerByEmail)
			grantee.EmailAddress = aws.String(g.ID)
		case "uri":
			grantee.Type = aws.String(s3.TypeGroup)
			grantee.URI = aws.String(g.ID)
		}

		grants = append(grants, &s3.Grant{
			Grantee:    &grantee,
			Permission: aws.String(g.Permissions),
		})
	}

	if ev.ACL == "" {
		grt, err := ev.getACL()
		if err != nil {
			return err
		}

		params.AccessControlPolicy = &s3.AccessControlPolicy{
			Grants: grants,
			Owner:  grt.Owner,
		}
	}

	_, err := s3client.PutBucketAcl(params)
	if err != nil {
		return err
	}

	return ev.setTags()
}

// Delete : Deletes a nat object on aws
func (ev *Event) Delete() error {
	s3client := ev.getS3Client()
	params := &s3.DeleteBucketInput{
		Bucket: aws.String(ev.Name),
	}
	_, err := s3client.DeleteBucket(params)

	return err
}

// Get : Gets a nat object on aws
func (ev *Event) Get() error {
	return errors.New(ev.Subject + " not supported")
}

func (ev *Event) getS3Client() *s3.S3 {
	creds, _ := credentials.NewStaticCredentials(ev.AWSAccessKeyID, ev.AWSSecretAccessKey, ev.CryptoKey)
	s3client := s3.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})
	return s3client
}

func (ev *Event) getACL() (*s3.GetBucketAclOutput, error) {
	s3client := ev.getS3Client()

	params := &s3.GetBucketAclInput{
		Bucket: aws.String(ev.Name),
	}

	resp, err := s3client.GetBucketAcl(params)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (ev *Event) setTags() error {
	svc := ev.getS3Client()

	req := &s3.PutBucketTaggingInput{
		Bucket: &ev.Name,
	}

	tags := s3.Tagging{}

	for key, val := range ev.Tags {
		tags.TagSet = append(tags.TagSet, &s3.Tag{
			Key:   &key,
			Value: &val,
		})
	}

	req.Tagging = &tags

	_, err := svc.PutBucketTagging(req)

	return err
}
