// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package ec2 provides a client to make API requests to Amazon Elastic Compute Cloud.
package ec2

import (
	"context"
	"fmt"
	"strings"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

const (
	defaultForAZFilterName   = "default-for-az"
	internetGatewayIDPrefix  = "igw-"
	cloudFrontPrefixListName = "com.amazonaws.global.cloudfront.origin-facing"

	// FmtTagFilter is the filter name format for tag filters
	FmtTagFilter = "tag:%s"
	tagKeyFilter = "tag-key"
)

var (
	// FilterForDefaultVPCSubnets is a pre-defined filter for the default subnets at the availability zone.
	FilterForDefaultVPCSubnets = Filter{
		Name:   defaultForAZFilterName,
		Values: []string{"true"},
	}
)

type api interface {
	DescribeSubnets(ctx context.Context, input *ec2.DescribeSubnetsInput, opts ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	DescribeSecurityGroups(ctx context.Context, input *ec2.DescribeSecurityGroupsInput, opts ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
	DescribeVpcs(ctx context.Context, input *ec2.DescribeVpcsInput, opts ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
	DescribeVpcAttribute(ctx context.Context, input *ec2.DescribeVpcAttributeInput, opts ...func(*ec2.Options)) (*ec2.DescribeVpcAttributeOutput, error)
	DescribeNetworkInterfaces(ctx context.Context, input *ec2.DescribeNetworkInterfacesInput, opts ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error)
	DescribeRouteTables(ctx context.Context, input *ec2.DescribeRouteTablesInput, opts ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error)
	DescribeAvailabilityZones(ctx context.Context, input *ec2.DescribeAvailabilityZonesInput, opts ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error)
	DescribeManagedPrefixLists(ctx context.Context, input *ec2.DescribeManagedPrefixListsInput, opts ...func(*ec2.Options)) (*ec2.DescribeManagedPrefixListsOutput, error)
}

// Filter contains the name and values of a filter.
type Filter struct {
	// Name of a filter that will be applied to subnets,
	// for available filter names see: https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeSubnets.html.
	Name string
	// Value of the filter.
	Values []string
}

// FilterForTags takes a key and optional values to construct an EC2 filter.
func FilterForTags(key string, values ...string) Filter {
	if len(values) == 0 {
		return Filter{Name: tagKeyFilter, Values: []string{key}}
	}
	return Filter{
		Name:   fmt.Sprintf(FmtTagFilter, key),
		Values: values,
	}
}

// EC2 wraps an AWS EC2 client.
type EC2 struct {
	client api
}

// New returns a EC2 configured against the input config.
func New(cfg awsv2.Config) *EC2 {
	return &EC2{
		client: ec2.NewFromConfig(cfg),
	}
}

// Resource contains the ID and name of a EC2 resource.
type Resource struct {
	ID   string
	Name string
}

// VPC contains the ID and name of a VPC.
type VPC struct {
	Resource
}

// Subnet contains the ID and name of a subnet.
type Subnet struct {
	Resource
	CIDRBlock string
}

// AZ represents an availability zone.
type AZ Resource

// String formats the elements of a VPC into a display-ready string.
// For example: VPCResource{"ID": "vpc-0576efeea396efee2", "Name": "video-store-test"}
// will return "vpc-0576efeea396efee2 (copilot-video-store-test)".
// while VPCResource{"ID": "subnet-018ccb78d353cec9b", "Name": "public-subnet-1"}
// will return "subnet-018ccb78d353cec9b (public-subnet-1)"
func (r *Resource) String() string {
	if r.Name != "" {
		return fmt.Sprintf("%s (%s)", r.ID, r.Name)
	}
	return r.ID
}

// ExtractVPC extracts the vpc ID from the resource display string.
// For example: vpc-0576efeea396efee2 (copilot-video-store-test)
// will return VPC{ID: "vpc-0576efeea396efee2", Name: "copilot-video-store-test"}.
func ExtractVPC(label string) (*VPC, error) {
	resource, err := extractResource(label)
	if err != nil {
		return nil, err
	}
	return &VPC{
		Resource: *resource,
	}, nil
}

// ExtractSubnet extracts the subnet ID from the resource display string.
func ExtractSubnet(label string) (*Subnet, error) {
	resource, err := extractResource(label)
	if err != nil {
		return nil, err
	}
	return &Subnet{
		Resource: *resource,
	}, nil
}

func extractResource(label string) (*Resource, error) {
	if label == "" {
		return nil, fmt.Errorf("extract resource ID from string: %s", label)
	}
	splitResource := strings.SplitN(label, " ", 2)
	// TODO: switch to regex to make more robust
	var name string
	if len(splitResource) == 2 {
		name = strings.Trim(splitResource[1], "()")
	}
	return &Resource{
		ID:   splitResource[0],
		Name: name,
	}, nil
}

// PublicIP returns the public ip associated with the network interface.
func (c *EC2) PublicIP(eni string) (string, error) {
	return c.PublicIPWithContext(context.Background(), eni)
}

// PublicIPWithContext returns the public IP associated with the network interface using ctx.
func (c *EC2) PublicIPWithContext(ctx context.Context, eni string) (string, error) {
	response, err := c.client.DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{
		NetworkInterfaceIds: []string{eni},
	})
	if err != nil {
		return "", fmt.Errorf("describe network interface with ENI %s: %w", eni, err)
	}

	// `response.NetworkInterfaces` contains at least one result; if no matching ENI is found, the API call will return
	// an error instead of an empty list of `NetworkInterfaces` (https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeNetworkInterfaces.html)
	association := response.NetworkInterfaces[0].Association
	if association == nil {
		return "", fmt.Errorf("no association information found for ENI %s", eni)
	}

	return awsv2.ToString(association.PublicIp), nil
}

// ListVPCs returns names and IDs (or just IDs, if Name tag does not exist) of all VPCs.
func (c *EC2) ListVPCs() ([]VPC, error) {
	return c.ListVPCsWithContext(context.Background())
}

// ListVPCsWithContext returns names and IDs of all VPCs using ctx for every page.
func (c *EC2) ListVPCsWithContext(ctx context.Context) ([]VPC, error) {
	var ec2vpcs []types.Vpc
	response, err := c.client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{})
	if err != nil {
		return nil, fmt.Errorf("describe VPCs: %w", err)
	}
	ec2vpcs = append(ec2vpcs, response.Vpcs...)

	for response.NextToken != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		response, err = c.client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
			NextToken: response.NextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("describe VPCs: %w", err)
		}
		ec2vpcs = append(ec2vpcs, response.Vpcs...)
	}
	var vpcs []VPC
	for _, vpc := range ec2vpcs {
		var name string
		for _, tag := range vpc.Tags {
			if awsv2.ToString(tag.Key) == "Name" {
				name = awsv2.ToString(tag.Value)
			}
		}
		vpcs = append(vpcs, VPC{
			Resource: Resource{
				ID:   awsv2.ToString(vpc.VpcId),
				Name: name,
			},
		})
	}
	return vpcs, nil
}

// ListAZs returns the list of opted-in and available availability zones.
func (c *EC2) ListAZs() ([]AZ, error) {
	return c.ListAZsWithContext(context.Background())
}

// ListAZsWithContext returns opted-in and available availability zones using ctx.
func (c *EC2) ListAZsWithContext(ctx context.Context) ([]AZ, error) {
	resp, err := c.client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{
		Filters: []types.Filter{
			{
				Name:   awsv2.String("zone-type"),
				Values: []string{"availability-zone"},
			},
			{
				Name:   awsv2.String("state"),
				Values: []string{"available"},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("describe availability zones: %w", err)
	}
	var out []AZ
	for _, az := range resp.AvailabilityZones {
		out = append(out, AZ{
			ID:   awsv2.ToString(az.ZoneId),
			Name: awsv2.ToString(az.ZoneName),
		})
	}
	return out, nil
}

// HasDNSSupport returns if DNS resolution is enabled for the VPC.
func (c *EC2) HasDNSSupport(vpcID string) (bool, error) {
	return c.HasDNSSupportWithContext(context.Background(), vpcID)
}

// HasDNSSupportWithContext returns whether DNS resolution is enabled for the VPC using ctx.
func (c *EC2) HasDNSSupportWithContext(ctx context.Context, vpcID string) (bool, error) {
	resp, err := c.client.DescribeVpcAttribute(ctx, &ec2.DescribeVpcAttributeInput{
		VpcId:     awsv2.String(vpcID),
		Attribute: types.VpcAttributeNameEnableDnsSupport,
	})
	if err != nil {
		return false, fmt.Errorf("describe %s attribute for VPC %s: %w", types.VpcAttributeNameEnableDnsSupport, vpcID, err)
	}
	return awsv2.ToBool(resp.EnableDnsSupport.Value), nil
}

// VPCSubnets are all subnets within a VPC.
type VPCSubnets struct {
	Public  []Subnet
	Private []Subnet
}

// ListVPCSubnets lists all subnets with a given VPC ID. Note that public subnets
// are subnets associated with an internet gateway through a route table.
// And the rest of the subnets are private.
func (c *EC2) ListVPCSubnets(vpcID string) (*VPCSubnets, error) {
	return c.ListVPCSubnetsWithContext(context.Background(), vpcID)
}

// ListVPCSubnetsWithContext lists all subnets in a VPC using ctx for every request.
func (c *EC2) ListVPCSubnetsWithContext(ctx context.Context, vpcID string) (*VPCSubnets, error) {
	vpcFilter := Filter{
		Name:   "vpc-id",
		Values: []string{vpcID},
	}
	routeTables, err := c.routeTables(ctx, vpcFilter)
	if err != nil {
		return nil, err
	}
	rtIndex := indexRouteTables(routeTables)

	var publicSubnets, privateSubnets []Subnet
	respSubnets, err := c.subnets(ctx, vpcFilter)
	if err != nil {
		return nil, err
	}
	for _, subnet := range respSubnets {
		var name string
		for _, tag := range subnet.Tags {
			if awsv2.ToString(tag.Key) == "Name" {
				name = awsv2.ToString(tag.Value)
			}
		}
		s := Subnet{
			Resource: Resource{
				ID:   awsv2.ToString(subnet.SubnetId),
				Name: name,
			},
			CIDRBlock: awsv2.ToString(subnet.CidrBlock),
		}
		if rtIndex.IsPublicSubnet(s.ID) {
			publicSubnets = append(publicSubnets, s)
		} else {
			privateSubnets = append(privateSubnets, s)
		}
	}
	return &VPCSubnets{
		Public:  publicSubnets,
		Private: privateSubnets,
	}, nil
}

// SubnetIDs finds the subnet IDs with optional filters.
func (c *EC2) SubnetIDs(filters ...Filter) ([]string, error) {
	return c.SubnetIDsWithContext(context.Background(), filters...)
}

// SubnetIDsWithContext finds subnet IDs with optional filters using ctx.
func (c *EC2) SubnetIDsWithContext(ctx context.Context, filters ...Filter) ([]string, error) {
	subnets, err := c.subnets(ctx, filters...)
	if err != nil {
		return nil, err
	}

	subnetIDs := make([]string, len(subnets))
	for idx, subnet := range subnets {
		subnetIDs[idx] = awsv2.ToString(subnet.SubnetId)
	}
	return subnetIDs, nil
}

// SecurityGroups finds the security group IDs with optional filters.
func (c *EC2) SecurityGroups(filters ...Filter) ([]string, error) {
	return c.SecurityGroupsWithContext(context.Background(), filters...)
}

// SecurityGroupsWithContext finds security group IDs with optional filters using ctx.
func (c *EC2) SecurityGroupsWithContext(ctx context.Context, filters ...Filter) ([]string, error) {
	inputFilters := toEC2Filter(filters)

	response, err := c.client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: inputFilters,
	})

	if err != nil {
		return nil, fmt.Errorf("describe security groups: %w", err)
	}

	securityGroups := make([]string, len(response.SecurityGroups))
	for idx, sg := range response.SecurityGroups {
		securityGroups[idx] = awsv2.ToString(sg.GroupId)
	}
	return securityGroups, nil
}

func (c *EC2) subnets(ctx context.Context, filters ...Filter) ([]types.Subnet, error) {
	inputFilters := toEC2Filter(filters)
	var subnets []types.Subnet
	response, err := c.client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: inputFilters,
	})
	if err != nil {
		return nil, fmt.Errorf("describe subnets: %w", err)
	}
	subnets = append(subnets, response.Subnets...)
	for response.NextToken != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		response, err = c.client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
			Filters:   inputFilters,
			NextToken: response.NextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("describe subnets: %w", err)
		}
		subnets = append(subnets, response.Subnets...)
	}
	if len(subnets) == 0 {
		return nil, fmt.Errorf("cannot find any subnets")
	}
	return subnets, nil
}

func (c *EC2) routeTables(ctx context.Context, filters ...Filter) ([]types.RouteTable, error) {
	var routeTables []types.RouteTable
	input := &ec2.DescribeRouteTablesInput{
		Filters: toEC2Filter(filters),
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resp, err := c.client.DescribeRouteTables(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("describe route tables: %w", err)
		}
		routeTables = append(routeTables, resp.RouteTables...)
		if resp.NextToken == nil {
			break
		}
		input.NextToken = resp.NextToken
	}
	return routeTables, nil
}

func toEC2Filter(filters []Filter) []types.Filter {
	var ec2Filter []types.Filter
	for _, filter := range filters {
		ec2Filter = append(ec2Filter, types.Filter{
			Name:   awsv2.String(filter.Name),
			Values: filter.Values,
		})
	}
	return ec2Filter
}

type routeTable types.RouteTable

// IsMain returns true if the route table is the default route table for the VPC.
// If a subnet is not associated with a particular route table, then it will default to the main route table.
func (rt *routeTable) IsMain() bool {
	for _, association := range rt.Associations {
		if awsv2.ToBool(association.Main) {
			return true
		}
	}
	return false
}

// HasIGW returns true if the route table has a route to an internet gateway.
func (rt *routeTable) HasIGW() bool {
	for _, route := range rt.Routes {
		if strings.HasPrefix(awsv2.ToString(route.GatewayId), internetGatewayIDPrefix) {
			return true
		}
	}
	return false
}

// AssociatedSubnets returns the list of subnet IDs associated with the route table.
func (rt *routeTable) AssociatedSubnets() []string {
	var subnetIDs []string
	for _, association := range rt.Associations {
		if association.SubnetId == nil {
			continue
		}
		subnetIDs = append(subnetIDs, awsv2.ToString(association.SubnetId))
	}
	return subnetIDs
}

// routeTableIndex holds cached data to quickly return information about route tables in a VPC.
type routeTableIndex struct {
	// Route table that subnets default to. There is always one main table in the VPC.
	mainTable *routeTable

	// Explicit route table association for a subnet. A subnet can only be associated to one route table.
	routeTableForSubnet map[string]*routeTable
}

func indexRouteTables(tables []types.RouteTable) *routeTableIndex {
	index := &routeTableIndex{
		routeTableForSubnet: make(map[string]*routeTable),
	}
	for i := range tables { // Index all properties in a single pass.
		table := (*routeTable)(&tables[i])

		for _, subnetID := range table.AssociatedSubnets() {
			index.routeTableForSubnet[subnetID] = table
		}

		if table.IsMain() {
			index.mainTable = table
		}
	}
	return index
}

// IsPublicSubnet returns true if the subnet has a route to an internet gateway.
// We consider the subnet to have internet access if there is an explicit route in the route table to an internet gateway.
// Or if there is an implicit route, where the subnet defaults to the main route table with an internet gateway.
func (idx *routeTableIndex) IsPublicSubnet(subnetID string) bool {
	rt, ok := idx.routeTableForSubnet[subnetID]
	if ok {
		return rt.HasIGW()
	}
	return idx.mainTable.HasIGW()
}

// managedPrefixList returns the DescribeManagedPrefixListsOutput of a query by name.
func (c *EC2) managedPrefixList(ctx context.Context, prefixListName string) (*ec2.DescribeManagedPrefixListsOutput, error) {
	prefixListOutput, err := c.client.DescribeManagedPrefixLists(ctx, &ec2.DescribeManagedPrefixListsInput{
		Filters: []types.Filter{
			{
				Name:   awsv2.String("prefix-list-name"),
				Values: []string{prefixListName},
			},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("describe managed prefix list with name %s: %w", prefixListName, err)
	}

	return prefixListOutput, nil
}

// CloudFrontManagedPrefixListID returns the PrefixListId of the associated cloudfront prefix list as a *string.
func (c *EC2) CloudFrontManagedPrefixListID() (string, error) {
	return c.CloudFrontManagedPrefixListIDWithContext(context.Background())
}

// CloudFrontManagedPrefixListIDWithContext returns the CloudFront managed prefix list ID using ctx.
func (c *EC2) CloudFrontManagedPrefixListIDWithContext(ctx context.Context) (string, error) {
	prefixListsOutput, err := c.managedPrefixList(ctx, cloudFrontPrefixListName)

	if err != nil {
		return "", err
	}

	var ids []string
	for _, v := range prefixListsOutput.PrefixLists {
		ids = append(ids, *v.PrefixListId)
	}

	if len(ids) == 0 {
		return "", fmt.Errorf("cannot find any prefix list with name: %s", cloudFrontPrefixListName)
	}

	if len(ids) > 1 {
		return "", fmt.Errorf("found more than one prefix list with the name %s: %v", cloudFrontPrefixListName, ids)
	}

	return ids[0], nil
}
