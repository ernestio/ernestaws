/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package firewall

import (
	"encoding/json"
	"errors"
	"log"
	"reflect"

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
	// ErrSGAWSIDInvalid ...
	ErrSGAWSIDInvalid = errors.New("Security Group aws id invalid")
	// ErrSGNameInvalid ...
	ErrSGNameInvalid = errors.New("Security Group name invalid")
	// ErrSGRulesInvalid ...
	ErrSGRulesInvalid = errors.New("Security Group must contain rules")
	// ErrSGRuleIPInvalid ...
	ErrSGRuleIPInvalid = errors.New("Security Group rule ip invalid")
	// ErrSGRuleProtocolInvalid ...
	ErrSGRuleProtocolInvalid = errors.New("Security Group rule protocol invalid")
	// ErrSGRuleFromPortInvalid ...
	ErrSGRuleFromPortInvalid = errors.New("Security Group rule from port invalid")
	// ErrSGRuleToPortInvalid ...
	ErrSGRuleToPortInvalid = errors.New("Security Group rule to port invalid")
)

type rule struct {
	IP       string `json:"ip"`
	FromPort int64  `json:"from_port"`
	ToPort   int64  `json:"to_port"`
	Protocol string `json:"protocol"`
}

// Event stores the template data
type Event struct {
	UUID                  string `json:"_uuid"`
	BatchID               string `json:"_batch_id"`
	ProviderType          string `json:"_type"`
	VPCID                 string `json:"vpc_id"`
	DatacenterRegion      string `json:"datacenter_region"`
	DatacenterAccessKey   string `json:"datacenter_secret"`
	DatacenterAccessToken string `json:"datacenter_token"`
	NetworkAWSID          string `json:"network_aws_id"`
	SecurityGroupAWSID    string `json:"security_group_aws_id,omitempty"`
	SecurityGroupName     string `json:"name"`
	SecurityGroupRules    struct {
		Ingress []rule `json:"ingress"`
		Egress  []rule `json:"egress"`
	} `json:"rules"`
	ErrorMessage string `json:"error,omitempty"`
	Subject      string `json:"-"`
	Body         []byte `json:"-"`
	CryptoKey    []byte `json:"-"`
}

// New : Constructor
func New(subject string, body, cryptoKey []byte) ernestaws.Event {
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

	if ev.Subject != "firewall.create.aws" {
		if ev.SecurityGroupAWSID == "" {
			return ErrSGAWSIDInvalid
		}
	}
	if ev.Subject != "firewall.delete.aws" {
		if ev.SecurityGroupName == "" {
			return ErrSGNameInvalid
		}

		if len(ev.SecurityGroupRules.Egress) < 1 && len(ev.SecurityGroupRules.Egress) < 1 {
			return ErrSGRulesInvalid
		}
		for _, rule := range ev.SecurityGroupRules.Ingress {
			if rule.IP == "" {
				return ErrSGRuleIPInvalid
			}
			if rule.Protocol == "" {
				return ErrSGRuleProtocolInvalid
			}
			if rule.FromPort < 0 || rule.FromPort > 65535 {
				return ErrSGRuleFromPortInvalid
			}
			if rule.ToPort < 0 || rule.ToPort > 65535 {
				return ErrSGRuleToPortInvalid
			}
		}

		for _, rule := range ev.SecurityGroupRules.Egress {
			if rule.IP == "" {
				return ErrSGRuleIPInvalid
			}
			if rule.Protocol == "" {
				return ErrSGRuleProtocolInvalid
			}
			if rule.FromPort < 0 || rule.FromPort > 65535 {
				return ErrSGRuleFromPortInvalid
			}
			if rule.ToPort < 0 || rule.ToPort > 65535 {
				return ErrSGRuleToPortInvalid
			}
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

	// Create SecurityGroup
	req := ec2.CreateSecurityGroupInput{
		VpcId:       aws.String(ev.VPCID),
		GroupName:   aws.String(ev.SecurityGroupName),
		Description: aws.String("Rules for: " + ev.SecurityGroupName),
	}

	resp, err := svc.CreateSecurityGroup(&req)
	if err != nil {
		return err
	}

	ev.SecurityGroupAWSID = *resp.GroupId

	// Remove default rule
	err = ev.removeDefaultRule(svc, resp.GroupId)
	if err != nil {
		return err
	}

	// Authorize Ingress
	if len(ev.SecurityGroupRules.Ingress) > 0 {
		iReq := ec2.AuthorizeSecurityGroupIngressInput{
			GroupId:       aws.String(ev.SecurityGroupAWSID),
			IpPermissions: ev.buildPermissions(ev.SecurityGroupRules.Ingress),
		}

		_, err = svc.AuthorizeSecurityGroupIngress(&iReq)
		if err != nil {
			return err
		}
	}

	// Authorize Egress
	if len(ev.SecurityGroupRules.Egress) > 0 {
		eReq := ec2.AuthorizeSecurityGroupEgressInput{
			GroupId:       aws.String(ev.SecurityGroupAWSID),
			IpPermissions: ev.buildPermissions(ev.SecurityGroupRules.Egress),
		}

		_, err = svc.AuthorizeSecurityGroupEgress(&eReq)
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

	sg, err := ev.securityGroupByID(svc, ev.SecurityGroupAWSID)
	if err != nil {
		return err
	}

	// generate the new rulesets
	newIngressRules := ev.buildPermissions(ev.SecurityGroupRules.Ingress)
	newEgressRules := ev.buildPermissions(ev.SecurityGroupRules.Egress)

	// generate the rules to remove
	revokeIngressRules := ev.buildRevokePermissions(sg.IpPermissions, newIngressRules)
	revokeEgressRules := ev.buildRevokePermissions(sg.IpPermissionsEgress, newEgressRules)

	// remove already existing rules from the new ruleset
	newIngressRules = ev.deduplicateRules(newIngressRules, sg.IpPermissions)
	newEgressRules = ev.deduplicateRules(newEgressRules, sg.IpPermissionsEgress)

	// Revoke Ingress
	if len(revokeIngressRules) > 0 {
		iReq := ec2.RevokeSecurityGroupIngressInput{
			GroupId:       aws.String(ev.SecurityGroupAWSID),
			IpPermissions: revokeIngressRules,
		}

		_, err := svc.RevokeSecurityGroupIngress(&iReq)
		if err != nil {
			return err
		}
	}

	// Revoke Egress
	if len(revokeEgressRules) > 0 {
		eReq := ec2.RevokeSecurityGroupEgressInput{
			GroupId:       aws.String(ev.SecurityGroupAWSID),
			IpPermissions: revokeEgressRules,
		}
		_, err := svc.RevokeSecurityGroupEgress(&eReq)
		if err != nil {
			return err
		}
	}

	// Authorize Ingress
	if len(newIngressRules) > 0 {
		iReq := ec2.AuthorizeSecurityGroupIngressInput{
			GroupId:       aws.String(ev.SecurityGroupAWSID),
			IpPermissions: newIngressRules,
		}

		_, err := svc.AuthorizeSecurityGroupIngress(&iReq)
		if err != nil {
			return err
		}
	}

	// Authorize Egress
	if len(newEgressRules) > 0 {
		eReq := ec2.AuthorizeSecurityGroupEgressInput{
			GroupId:       aws.String(ev.SecurityGroupAWSID),
			IpPermissions: newEgressRules,
		}

		_, err := svc.AuthorizeSecurityGroupEgress(&eReq)
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

	req := ec2.DeleteSecurityGroupInput{
		GroupId: aws.String(ev.SecurityGroupAWSID),
	}

	_, err := svc.DeleteSecurityGroup(&req)
	if err != nil {
		return err
	}

	return nil
}

// Get : Gets a nat object on aws
func (ev *Event) Get() error {
	return errors.New(ev.Subject + " not supported")
}

func (ev *Event) removeDefaultRule(svc *ec2.EC2, sgID *string) error {
	perms := []*ec2.IpPermission{
		&ec2.IpPermission{
			FromPort:   aws.Int64(0),
			ToPort:     aws.Int64(65535),
			IpProtocol: aws.String("-1"),
			IpRanges: []*ec2.IpRange{
				&ec2.IpRange{CidrIp: aws.String("0.0.0.0/0")},
			},
		},
	}

	eReq := ec2.RevokeSecurityGroupEgressInput{
		GroupId:       sgID,
		IpPermissions: perms,
	}
	_, err := svc.RevokeSecurityGroupEgress(&eReq)
	return err
}

func (ev *Event) securityGroupByID(svc *ec2.EC2, id string) (*ec2.SecurityGroup, error) {
	f := []*ec2.Filter{
		&ec2.Filter{
			Name:   aws.String("group-id"),
			Values: []*string{aws.String(id)},
		},
	}

	req := ec2.DescribeSecurityGroupsInput{Filters: f}
	resp, err := svc.DescribeSecurityGroups(&req)
	if err != nil {
		return nil, err
	}

	if len(resp.SecurityGroups) != 1 {
		return nil, errors.New("Could not find security group")
	}

	return resp.SecurityGroups[0], nil
}

func (ev *Event) buildPermissions(rules []rule) []*ec2.IpPermission {
	var perms []*ec2.IpPermission
	for _, rule := range rules {
		p := ec2.IpPermission{
			FromPort:   aws.Int64(rule.FromPort),
			ToPort:     aws.Int64(rule.ToPort),
			IpProtocol: aws.String(rule.Protocol),
		}
		ip := ec2.IpRange{CidrIp: aws.String(rule.IP)}
		p.IpRanges = append(p.IpRanges, &ip)
		perms = append(perms, &p)
	}
	return perms
}

func (ev *Event) buildRevokePermissions(old, new []*ec2.IpPermission) []*ec2.IpPermission {
	var revoked []*ec2.IpPermission
	for _, rule := range old {
		if ev.ruleExists(rule, new) != true {
			revoked = append(revoked, rule)
		}
	}
	return revoked
}

func (ev *Event) deduplicateRules(rules, old []*ec2.IpPermission) []*ec2.IpPermission {
	for i := len(rules) - 1; i >= 0; i-- {
		if ev.ruleExists(rules[i], old) {
			rules = append(rules[:i], rules[i+1:]...)
		}
	}
	return rules
}

func (ev *Event) ruleExists(rule *ec2.IpPermission, ruleset []*ec2.IpPermission) bool {
	for _, r := range ruleset {
		if reflect.DeepEqual(*r, *rule) {
			return true
		}
	}
	return false
}
