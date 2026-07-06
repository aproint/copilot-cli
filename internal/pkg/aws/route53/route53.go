// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package route53 provides functionality to manipulate route53 primitives.
package route53

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

const (
	// See https://docs.aws.amazon.com/general/latest/gr/r53.html
	// For Route53 API endpoint, "Route 53 in AWS Regions other than the Beijing and Ningxia Regions: specify us-east-1 as the Region."
	route53Region = "us-east-1"
)

type api interface {
	ListHostedZonesByName(context.Context, *route53.ListHostedZonesByNameInput, ...func(*route53.Options)) (*route53.ListHostedZonesByNameOutput, error)
	ListResourceRecordSets(context.Context, *route53.ListResourceRecordSetsInput, ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error)
}

type nameserverResolver interface {
	LookupNS(ctx context.Context, name string) ([]*net.NS, error)
}

// Route53 wraps an Route53 client.
type Route53 struct {
	client api
	dns    nameserverResolver

	hostedZoneIDFor map[string]string
}

// New returns a Route53 struct configured against the input SDK v2 config.
func New(cfg awsv2.Config) *Route53 {
	cfg.Region = route53Region
	return &Route53{
		client:          route53.NewFromConfig(cfg),
		dns:             new(net.Resolver),
		hostedZoneIDFor: make(map[string]string),
	}
}

// PublicDomainHostedZoneID returns the public Hosted Zone ID of a domain.
func (r53 *Route53) PublicDomainHostedZoneID(domainName string) (string, error) {
	if id, ok := r53.hostedZoneIDFor[domainName]; ok {
		return id, nil
	}

	in := &route53.ListHostedZonesByNameInput{DNSName: awsv2.String(domainName)}
	resp, err := r53.client.ListHostedZonesByName(context.Background(), in)
	if err != nil {
		return "", fmt.Errorf("list hosted zone for %s: %w", domainName, err)
	}
	for {
		hostedZones := filterHostedZones(resp.HostedZones, matchesDomain(domainName), matchesPublic())
		if len(hostedZones) > 0 {
			// return the first match.
			id := strings.TrimPrefix(awsv2.ToString(hostedZones[0].Id), "/hostedzone/")
			r53.hostedZoneIDFor[domainName] = id
			return id, nil
		}
		if !resp.IsTruncated {
			return "", &ErrDomainHostedZoneNotFound{
				domainName: domainName,
			}
		}
		in = &route53.ListHostedZonesByNameInput{DNSName: resp.NextDNSName, HostedZoneId: resp.NextHostedZoneId}
		resp, err = r53.client.ListHostedZonesByName(context.Background(), in)
		if err != nil {
			return "", fmt.Errorf("list hosted zone for %s: %w", domainName, err)
		}
	}
}

// ValidateDomainOwnership returns nil if the NS records associated with the domain name matches the NS records of the
// route53 hosted zone for the domain.
// If there are missing NS records returns ErrUnmatchedNSRecords.
func (r53 *Route53) ValidateDomainOwnership(domainName string) error {
	hzID, err := r53.PublicDomainHostedZoneID(domainName)
	if err != nil {
		return err
	}

	wanted, err := r53.listHostedZoneNSRecords(domainName, hzID)
	if err != nil {
		return err
	}

	actual, err := r53.lookupNSRecords(domainName)
	if err != nil {
		return err
	}

	if !isStrictSubset(actual, wanted) {
		return &ErrUnmatchedNSRecords{
			domainName:   domainName,
			hostedZoneID: hzID,
			r53Records:   wanted,
			dnsRecords:   actual,
		}
	}
	return nil
}

func (r53 *Route53) listHostedZoneNSRecords(domainName, hostedZoneID string) ([]string, error) {
	out, err := r53.client.ListResourceRecordSets(context.Background(), &route53.ListResourceRecordSetsInput{
		HostedZoneId: awsv2.String(hostedZoneID),
	})
	if err != nil {
		return nil, fmt.Errorf("list resource record sets for hosted zone ID %q: %w", hostedZoneID, err)
	}
	var records []string
	for _, set := range out.ResourceRecordSets {
		if set.Type != types.RRTypeNs {
			continue
		}
		if name := awsv2.ToString(set.Name); !(name == domainName || name == domainName+".") /* filter only for parent domain */ {
			continue
		}
		for _, record := range set.ResourceRecords {
			records = append(records, cleanNSRecord(awsv2.ToString(record.Value)))
		}
	}
	return records, nil
}

func (r53 *Route53) lookupNSRecords(domainName string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	nameservers, err := r53.dns.LookupNS(ctx, domainName)
	if err != nil {
		return nil, fmt.Errorf("look up NS records for domain %q: %w", domainName, err)
	}

	var records []string
	for _, nameserver := range nameservers {
		records = append(records, cleanNSRecord(nameserver.Host))
	}
	return records, nil
}

type filterZoneFunc func(types.HostedZone) bool

func filterHostedZones(zones []types.HostedZone, filterFuncs ...filterZoneFunc) []types.HostedZone {
	var hostedZones []types.HostedZone
	passesAllFilters := func(zone types.HostedZone) bool {
		for _, fn := range filterFuncs {
			if !fn(zone) {
				return false
			}
		}
		return true
	}
	for _, hostedZone := range zones {
		if passesAllFilters(hostedZone) {
			hostedZones = append(hostedZones, hostedZone)
		}
	}
	return hostedZones
}

func matchesDomain(domain string) filterZoneFunc {
	return func(z types.HostedZone) bool {
		// example.com. should match example.com
		return domain == awsv2.ToString(z.Name) || domain+"." == awsv2.ToString(z.Name)
	}
}

func matchesPublic() filterZoneFunc {
	return func(zone types.HostedZone) bool {
		if zone.Config == nil {
			return true
		}
		return !zone.Config.PrivateZone
	}
}

func cleanNSRecord(record string) string {
	if !strings.HasSuffix(record, ".") {
		return record
	}
	return record[:len(record)-1]
}

func isStrictSubset(subset, superset []string) bool {
	if len(subset) > len(superset) {
		return false
	}

	isMember := make(map[string]struct{}, len(superset))
	for _, item := range superset {
		isMember[item] = struct{}{}
	}

	for _, item := range subset {
		if _, ok := isMember[item]; !ok {
			return false
		}
	}
	return true
}
