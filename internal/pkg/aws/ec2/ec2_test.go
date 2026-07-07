// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package ec2

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aproint/copilot-cli/internal/pkg/aws/ec2/mocks"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

var (
	inAppEnvFilters = []Filter{
		{
			Name:   fmt.Sprintf(FmtTagFilter, "copilot-application"),
			Values: []string{"my-app"},
		},
		{
			Name:   fmt.Sprintf(FmtTagFilter, "copilot-environment"),
			Values: []string{"my-env"},
		},
	}

	subnet1 = types.Subnet{
		SubnetId:            awsv2.String("subnet-1"),
		MapPublicIpOnLaunch: awsv2.Bool(false),
	}
	subnet2 = types.Subnet{
		SubnetId:            awsv2.String("subnet-2"),
		MapPublicIpOnLaunch: awsv2.Bool(true),
	}
	subnet3 = types.Subnet{
		SubnetId:            awsv2.String("subnet-3"),
		MapPublicIpOnLaunch: awsv2.Bool(true),
	}
)

func TestEC2_FilterForTags(t *testing.T) {
	testCases := map[string]struct {
		inValues []string
		wanted   Filter
	}{
		"with no values": {
			wanted: Filter{
				Name:   "tag-key",
				Values: []string{"mockKey"},
			},
		},
		"with values": {
			inValues: []string{"foo", "bar"},
			wanted: Filter{
				Name:   "tag:mockKey",
				Values: []string{"foo", "bar"},
			},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			filter := FilterForTags("mockKey", tc.inValues...)
			require.Equal(t, tc.wanted, filter)
		})
	}
}

func TestEC2_extractResource(t *testing.T) {
	testCases := map[string]struct {
		displayString  string
		wantedError    error
		wantedResource *Resource
	}{
		"returns error if string is empty": {
			displayString: "",
			wantedError:   fmt.Errorf("extract resource ID from string: "),
		},
		"returns just the VPC ID if no name present": {
			displayString: "vpc-imagr8vpcstring",
			wantedError:   nil,
			wantedResource: &Resource{
				ID: "vpc-imagr8vpcstring",
			},
		},
		"returns both the VPC ID and name if both present": {
			displayString: "vpc-imagr8vpcstring (copilot-app-name-env)",
			wantedError:   nil,
			wantedResource: &Resource{
				ID:   "vpc-imagr8vpcstring",
				Name: "copilot-app-name-env",
			},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			resource, err := extractResource(tc.displayString)
			if tc.wantedError != nil {
				require.EqualError(t, tc.wantedError, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantedResource, resource)
			}
		})
	}
}

func TestEC2_ListVPC(t *testing.T) {
	testCases := map[string]struct {
		mockEC2Client func(m *mocks.Mockapi)

		wantedError error
		wantedVPC   []VPC
	}{
		"fail to describe vpcs": {
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeVpcs(gomock.Any(), gomock.Any()).Return(nil, errors.New("some error"))
			},
			wantedError: fmt.Errorf("describe VPCs: some error"),
		},
		"success": {
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeVpcs(gomock.Any(), &ec2.DescribeVpcsInput{}).Return(&ec2.DescribeVpcsOutput{
					Vpcs: []types.Vpc{
						{
							VpcId: awsv2.String("mockVPCID1"),
						},
					},
					NextToken: awsv2.String("mockNextToken"),
				}, nil)
				m.EXPECT().DescribeVpcs(gomock.Any(), &ec2.DescribeVpcsInput{
					NextToken: awsv2.String("mockNextToken"),
				}).Return(&ec2.DescribeVpcsOutput{
					Vpcs: []types.Vpc{
						{
							VpcId: awsv2.String("mockVPCID2"),
							Tags: []types.Tag{
								{
									Key:   awsv2.String("Name"),
									Value: awsv2.String("mockVPC2Name"),
								},
							},
						},
					},
				}, nil)
			},
			wantedVPC: []VPC{
				{
					Resource: Resource{
						ID: "mockVPCID1",
					},
				},
				{
					Resource: Resource{
						ID:   "mockVPCID2",
						Name: "mockVPC2Name",
					},
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockAPI := mocks.NewMockapi(ctrl)
			tc.mockEC2Client(mockAPI)

			ec2Client := EC2{
				client: mockAPI,
			}

			vpcs, err := ec2Client.ListVPCs()
			if tc.wantedError != nil {
				require.EqualError(t, tc.wantedError, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantedVPC, vpcs)
			}
		})
	}
}

func TestEC2_ListAZs(t *testing.T) {
	testCases := map[string]struct {
		mockClient func(m *mocks.Mockapi)

		wantedErr string
		wantedAZs []AZ
	}{
		"return wrapped error on unexpected call error": {
			mockClient: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeAvailabilityZones(gomock.Any(), gomock.Any()).Return(nil, errors.New("some error"))
			},
			wantedErr: "describe availability zones: some error",
		},
		"returns AZs that are available and opted-in": {
			mockClient: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeAvailabilityZones(gomock.Any(), &ec2.DescribeAvailabilityZonesInput{
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
				}).Return(&ec2.DescribeAvailabilityZonesOutput{
					AvailabilityZones: []types.AvailabilityZone{
						{
							GroupName:          awsv2.String("us-west-2"),
							NetworkBorderGroup: awsv2.String("us-west-2"),
							OptInStatus:        types.AvailabilityZoneOptInStatusOptInNotRequired,
							RegionName:         awsv2.String("us-west-2"),
							State:              types.AvailabilityZoneStateAvailable,
							ZoneId:             awsv2.String("usw2-az1"),
							ZoneName:           awsv2.String("us-west-2a"),
							ZoneType:           awsv2.String("availability-zone"),
						},
						{
							GroupName:          awsv2.String("us-west-2"),
							NetworkBorderGroup: awsv2.String("us-west-2"),
							OptInStatus:        types.AvailabilityZoneOptInStatusOptInNotRequired,
							RegionName:         awsv2.String("us-west-2"),
							State:              types.AvailabilityZoneStateAvailable,
							ZoneId:             awsv2.String("usw2-az2"),
							ZoneName:           awsv2.String("us-west-2b"),
							ZoneType:           awsv2.String("availability-zone"),
						},
					},
				}, nil)
			},
			wantedAZs: []AZ{
				{
					ID:   "usw2-az1",
					Name: "us-west-2a",
				},
				{
					ID:   "usw2-az2",
					Name: "us-west-2b",
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			m := mocks.NewMockapi(ctrl)
			tc.mockClient(m)
			ec2 := EC2{client: m}

			// WHEN
			azs, err := ec2.ListAZs()

			// THEN
			if tc.wantedErr != "" {
				require.EqualError(t, err, tc.wantedErr)
			} else {
				require.NoError(t, err)
				require.ElementsMatch(t, tc.wantedAZs, azs)
			}
		})
	}
}

func TestEC2_managedPrefixList(t *testing.T) {
	const (
		mockPrefixListName = "mockName"
		mockPrefixListId   = "mockId"
		mockNextToken      = "mockNextToken"
	)
	mockError := errors.New("some error")
	mockFilter := []types.Filter{
		{
			Name:   awsv2.String("prefix-list-name"),
			Values: []string{mockPrefixListName},
		},
	}
	mockPrefixList := []types.ManagedPrefixList{
		{
			PrefixListId: awsv2.String(mockPrefixListId),
		},
	}

	testCases := map[string]struct {
		mockEC2Client func(m *mocks.Mockapi)

		wantedError          error
		wantedErrorMsgPrefix string
		wantedList           *ec2.DescribeManagedPrefixListsOutput
	}{
		"query returns error": {
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeManagedPrefixLists(gomock.Any(), gomock.Any()).Return(nil, mockError)
			},
			wantedError: fmt.Errorf("describe managed prefix list with name %s: %w", mockPrefixListName, mockError),
		},
		"query returns Successfully": {
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeManagedPrefixLists(gomock.Any(), &ec2.DescribeManagedPrefixListsInput{
					Filters: mockFilter,
				}).Return(&ec2.DescribeManagedPrefixListsOutput{
					NextToken: awsv2.String(mockNextToken),
					PrefixLists: []types.ManagedPrefixList{
						{
							PrefixListId: awsv2.String(mockPrefixListId),
						},
					},
				}, nil)
			},
			wantedList: &ec2.DescribeManagedPrefixListsOutput{
				NextToken:   awsv2.String(mockNextToken),
				PrefixLists: mockPrefixList,
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockAPI := mocks.NewMockapi(ctrl)
			tc.mockEC2Client(mockAPI)

			ec2Client := EC2{
				client: mockAPI,
			}

			output, err := ec2Client.managedPrefixList(mockPrefixListName)
			if tc.wantedError != nil {
				require.EqualError(t, tc.wantedError, err.Error())
			} else if tc.wantedErrorMsgPrefix != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantedErrorMsgPrefix)
				return
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantedList, output, "managed prefix lists output must be equal")
			}
		})
	}
}

func TestEC2_CloudFrontManagedPrefixListId(t *testing.T) {
	const (
		mockPrefixListName = cloudFrontPrefixListName
		mockPrefixListId   = "mockId"
		mockNextToken      = "mockNextToken"
	)
	mockError := errors.New("some error")
	mockFilter := []types.Filter{
		{
			Name:   awsv2.String("prefix-list-name"),
			Values: []string{mockPrefixListName},
		},
	}

	testCases := map[string]struct {
		mockEC2Client func(m *mocks.Mockapi)

		wantedError          error
		wantedErrorMsgPrefix string
		wantedId             string
	}{
		"query returns error": {
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeManagedPrefixLists(gomock.Any(), gomock.Any()).Return(nil, mockError)
			},
			wantedError: fmt.Errorf("describe managed prefix list with name %s: %w", mockPrefixListName, mockError),
		},
		"query returns no prefix list ids": {
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeManagedPrefixLists(gomock.Any(), &ec2.DescribeManagedPrefixListsInput{
					Filters: mockFilter,
				}).Return(&ec2.DescribeManagedPrefixListsOutput{
					NextToken:   awsv2.String(mockNextToken),
					PrefixLists: []types.ManagedPrefixList{},
				}, nil)
			},
			wantedError: fmt.Errorf("cannot find any prefix list with name: %s", mockPrefixListName),
		},
		"query returns too many prefix list ids": {
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeManagedPrefixLists(gomock.Any(), &ec2.DescribeManagedPrefixListsInput{
					Filters: mockFilter,
				}).Return(&ec2.DescribeManagedPrefixListsOutput{
					NextToken: awsv2.String(mockNextToken),
					PrefixLists: []types.ManagedPrefixList{
						{
							PrefixListId: awsv2.String(mockPrefixListId),
						},
						{
							PrefixListId: awsv2.String(mockPrefixListId),
						},
					},
				}, nil)
			},
			wantedErrorMsgPrefix: `found more than one prefix list with the name `,
		},
		"query returns Successfully": {
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeManagedPrefixLists(gomock.Any(), &ec2.DescribeManagedPrefixListsInput{
					Filters: mockFilter,
				}).Return(&ec2.DescribeManagedPrefixListsOutput{
					NextToken: awsv2.String(mockNextToken),
					PrefixLists: []types.ManagedPrefixList{
						{
							PrefixListId: awsv2.String(mockPrefixListId),
						},
					},
				}, nil)
			},
			wantedId: mockPrefixListId,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockAPI := mocks.NewMockapi(ctrl)
			tc.mockEC2Client(mockAPI)

			ec2Client := EC2{
				client: mockAPI,
			}

			id, err := ec2Client.CloudFrontManagedPrefixListID()
			if tc.wantedError != nil {
				require.EqualError(t, tc.wantedError, err.Error())
			} else if tc.wantedErrorMsgPrefix != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantedErrorMsgPrefix)
				return
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantedId, id, "ids must be equal")
			}
		})
	}
}

func TestEC2_ListVPCSubnets(t *testing.T) {
	const (
		mockVPCID     = "mockVPC"
		mockNextToken = "mockNextToken"
	)
	mockfilter := []types.Filter{
		{
			Name:   awsv2.String("vpc-id"),
			Values: []string{mockVPCID},
		},
	}
	mockError := errors.New("some error")

	testCases := map[string]struct {
		mockEC2Client func(m *mocks.Mockapi)

		wantedError          error
		wantedPublicSubnets  []Subnet
		wantedPrivateSubnets []Subnet
	}{
		"fail to describe route tables": {
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeRouteTables(gomock.Any(), gomock.Any()).Return(nil, mockError)
			},
			wantedError: fmt.Errorf("describe route tables: some error"),
		},
		"fail to describe subnets": {
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeRouteTables(gomock.Any(), &ec2.DescribeRouteTablesInput{
					Filters: mockfilter,
				}).Return(&ec2.DescribeRouteTablesOutput{}, nil)
				m.EXPECT().DescribeSubnets(gomock.Any(), gomock.Any()).Return(nil, mockError)
			},
			wantedError: fmt.Errorf("describe subnets: some error"),
		},
		"can retrieve subnets explicitly associated with an internet gateway": {
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeRouteTables(gomock.Any(), &ec2.DescribeRouteTablesInput{
					Filters: mockfilter,
				}).Return(&ec2.DescribeRouteTablesOutput{
					RouteTables: []types.RouteTable{
						{
							Associations: []types.RouteTableAssociation{
								{
									SubnetId: awsv2.String("subnet1"),
								},
							},
							Routes: []types.Route{
								{
									GatewayId: awsv2.String("local"),
								},
							},
						},
					},
					NextToken: awsv2.String(mockNextToken),
				}, nil)
				m.EXPECT().DescribeRouteTables(gomock.Any(), &ec2.DescribeRouteTablesInput{
					Filters:   mockfilter,
					NextToken: awsv2.String(mockNextToken),
				}).Return(&ec2.DescribeRouteTablesOutput{
					RouteTables: []types.RouteTable{
						{
							Associations: []types.RouteTableAssociation{
								{
									SubnetId: awsv2.String("subnet2"),
								},
								{
									SubnetId: awsv2.String("subnet3"),
								},
							},
							Routes: []types.Route{
								{
									GatewayId: awsv2.String("igw-0333791c413f9e2d8"),
								},
							},
						},
					},
				}, nil)
				m.EXPECT().DescribeSubnets(gomock.Any(), &ec2.DescribeSubnetsInput{
					Filters: mockfilter,
				}).Return(&ec2.DescribeSubnetsOutput{
					Subnets: []types.Subnet{
						{
							SubnetId:  awsv2.String("subnet1"),
							CidrBlock: awsv2.String("10.0.0.0/24"),
						},
						{
							SubnetId:  awsv2.String("subnet2"),
							CidrBlock: awsv2.String("10.0.1.0/24"),
						},
						{
							SubnetId: awsv2.String("subnet3"),
							Tags: []types.Tag{
								{
									Key:   awsv2.String("Name"),
									Value: awsv2.String("mySubnet"),
								},
							},
							CidrBlock: awsv2.String("10.0.2.0/24"),
						},
					},
				}, nil)
			},
			wantedPublicSubnets: []Subnet{
				{
					Resource: Resource{
						ID: "subnet2",
					},
					CIDRBlock: "10.0.1.0/24",
				},
				{
					Resource: Resource{
						ID:   "subnet3",
						Name: "mySubnet",
					},
					CIDRBlock: "10.0.2.0/24",
				},
			},
			wantedPrivateSubnets: []Subnet{
				{
					Resource: Resource{
						ID: "subnet1",
					},
					CIDRBlock: "10.0.0.0/24",
				},
			},
		},
		"can retrieve subnets that are implicitly associated with an internet gateway": {
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeRouteTables(gomock.Any(), &ec2.DescribeRouteTablesInput{
					Filters: mockfilter,
				}).Return(&ec2.DescribeRouteTablesOutput{
					RouteTables: []types.RouteTable{
						{
							Associations: []types.RouteTableAssociation{
								{
									Main: awsv2.Bool(true),
								},
							},
							Routes: []types.Route{
								{
									GatewayId:            awsv2.String("local"),
									DestinationCidrBlock: awsv2.String("172.31.0.0/16"),
								},
								{
									GatewayId:            awsv2.String("igw-3542f24c"),
									DestinationCidrBlock: awsv2.String("0.0.0.0/0"),
								},
							},
						},
					},
				}, nil)

				m.EXPECT().DescribeSubnets(gomock.Any(), &ec2.DescribeSubnetsInput{
					Filters: mockfilter,
				}).Return(&ec2.DescribeSubnetsOutput{
					Subnets: []types.Subnet{
						{
							SubnetId:  awsv2.String("subnet1"),
							CidrBlock: awsv2.String("172.31.16.0/20"),
						},
						{
							SubnetId:  awsv2.String("subnet2"),
							CidrBlock: awsv2.String("172.31.48.0/20"),
						},
						{
							SubnetId:  awsv2.String("subnet3"),
							CidrBlock: awsv2.String("172.31.32.0/20"),
						},
					},
				}, nil)
			},
			wantedPublicSubnets: []Subnet{
				{
					Resource: Resource{
						ID: "subnet1",
					},
					CIDRBlock: "172.31.16.0/20",
				},
				{
					Resource: Resource{
						ID: "subnet2",
					},
					CIDRBlock: "172.31.48.0/20",
				},
				{
					Resource: Resource{
						ID: "subnet3",
					},
					CIDRBlock: "172.31.32.0/20",
				},
			},
			wantedPrivateSubnets: nil,
		},
		"prioritizes explicit route table association over implicit while detecting public subnets": {
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeRouteTables(gomock.Any(), &ec2.DescribeRouteTablesInput{
					Filters: mockfilter,
				}).Return(&ec2.DescribeRouteTablesOutput{
					RouteTables: []types.RouteTable{
						{
							Associations: []types.RouteTableAssociation{
								{
									Main:     awsv2.Bool(false),
									SubnetId: awsv2.String("subnet1"),
								},
							},
							Routes: []types.Route{
								{
									GatewayId:            awsv2.String("local"),
									DestinationCidrBlock: awsv2.String("172.31.0.0/16"),
								},
							},
						},
						{
							Associations: []types.RouteTableAssociation{
								{
									Main: awsv2.Bool(true),
								},
							},
							Routes: []types.Route{
								{
									GatewayId:            awsv2.String("local"),
									DestinationCidrBlock: awsv2.String("172.31.0.0/16"),
								},
								{
									GatewayId:            awsv2.String("igw-3542f24c"),
									DestinationCidrBlock: awsv2.String("0.0.0.0/0"),
								},
							},
						},
					},
				}, nil)

				m.EXPECT().DescribeSubnets(gomock.Any(), &ec2.DescribeSubnetsInput{
					Filters: mockfilter,
				}).Return(&ec2.DescribeSubnetsOutput{
					Subnets: []types.Subnet{
						{
							SubnetId:  awsv2.String("subnet1"),
							CidrBlock: awsv2.String("172.31.16.0/20"),
						},
						{
							SubnetId:  awsv2.String("subnet2"),
							CidrBlock: awsv2.String("172.31.48.0/20"),
						},
					},
				}, nil)
			},
			wantedPublicSubnets: []Subnet{
				{
					Resource: Resource{
						ID: "subnet2",
					},
					CIDRBlock: "172.31.48.0/20",
				},
			},
			wantedPrivateSubnets: []Subnet{
				{
					Resource: Resource{
						ID: "subnet1",
					},
					CIDRBlock: "172.31.16.0/20",
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockAPI := mocks.NewMockapi(ctrl)
			tc.mockEC2Client(mockAPI)

			ec2Client := EC2{
				client: mockAPI,
			}

			subnets, err := ec2Client.ListVPCSubnets(mockVPCID)
			if tc.wantedError != nil {
				require.EqualError(t, tc.wantedError, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantedPublicSubnets, subnets.Public, "public subnets must equal")
				require.Equal(t, tc.wantedPrivateSubnets, subnets.Private, "private subnets must equal")
			}
		})
	}
}

func TestEC2_PublicIP(t *testing.T) {
	testCases := map[string]struct {
		inENI         string
		mockEC2Client func(m *mocks.Mockapi)

		wantedIP  string
		wantedErr error
	}{
		"failed to describe network interfaces": {
			inENI: "eni-1",
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeNetworkInterfaces(gomock.Any(), &ec2.DescribeNetworkInterfacesInput{
					NetworkInterfaceIds: []string{"eni-1"},
				}).Return(nil, errors.New("some error"))
			},
			wantedErr: errors.New("describe network interface with ENI eni-1: some error"),
		},
		"no association information found": {
			inENI: "eni-1",
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeNetworkInterfaces(gomock.Any(), &ec2.DescribeNetworkInterfacesInput{
					NetworkInterfaceIds: []string{"eni-1"},
				}).Return(&ec2.DescribeNetworkInterfacesOutput{
					NetworkInterfaces: []types.NetworkInterface{
						{},
					},
				}, nil)
			},
			wantedErr: errors.New("no association information found for ENI eni-1"),
		},
		"successfully get public ip": {
			inENI: "eni-1",
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeNetworkInterfaces(gomock.Any(), &ec2.DescribeNetworkInterfacesInput{
					NetworkInterfaceIds: []string{"eni-1"},
				}).Return(&ec2.DescribeNetworkInterfacesOutput{
					NetworkInterfaces: []types.NetworkInterface{
						{
							Association: &types.NetworkInterfaceAssociation{
								PublicIp: awsv2.String("1.2.3"),
							},
						},
					},
				}, nil)
			},
			wantedIP: "1.2.3",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockAPI := mocks.NewMockapi(ctrl)
			tc.mockEC2Client(mockAPI)

			ec2Client := EC2{
				client: mockAPI,
			}

			out, err := ec2Client.PublicIP(tc.inENI)
			if tc.wantedErr != nil {
				require.EqualError(t, tc.wantedErr, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantedIP, out)
			}
		})
	}
}

func TestEC2_SubnetIDs(t *testing.T) {
	mockNextToken := awsv2.String("mockNextToken")
	testCases := map[string]struct {
		inFilter []Filter

		mockEC2Client func(m *mocks.Mockapi)

		wantedError error
		wantedARNs  []string
	}{
		"failed to get subnets": {
			inFilter: inAppEnvFilters,
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeSubnets(gomock.Any(), &ec2.DescribeSubnetsInput{
					Filters: toEC2Filter(inAppEnvFilters),
				}).Return(nil, errors.New("error describing subnets"))
			},
			wantedError: fmt.Errorf("describe subnets: error describing subnets"),
		},
		"cannot get any subnets": {
			inFilter: inAppEnvFilters,
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeSubnets(gomock.Any(), &ec2.DescribeSubnetsInput{
					Filters: toEC2Filter(inAppEnvFilters),
				}).Return(&ec2.DescribeSubnetsOutput{
					Subnets: []types.Subnet{},
				}, nil)
			},
			wantedError: fmt.Errorf("cannot find any subnets"),
		},
		"successfully get subnets": {
			inFilter: inAppEnvFilters,
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeSubnets(gomock.Any(), &ec2.DescribeSubnetsInput{
					Filters: toEC2Filter(inAppEnvFilters),
				}).Return(&ec2.DescribeSubnetsOutput{
					Subnets: []types.Subnet{
						subnet1, subnet2,
					},
				}, nil)
			},
			wantedARNs: []string{"subnet-1", "subnet-2"},
		},
		"successfully get subnets with pagination": {
			inFilter: inAppEnvFilters,
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeSubnets(gomock.Any(), &ec2.DescribeSubnetsInput{
					Filters: toEC2Filter(inAppEnvFilters),
				}).Return(&ec2.DescribeSubnetsOutput{
					Subnets: []types.Subnet{
						subnet1, subnet2,
					},
					NextToken: mockNextToken,
				}, nil)
				m.EXPECT().DescribeSubnets(gomock.Any(), &ec2.DescribeSubnetsInput{
					Filters:   toEC2Filter(inAppEnvFilters),
					NextToken: mockNextToken,
				}).Return(&ec2.DescribeSubnetsOutput{
					Subnets: []types.Subnet{
						subnet3,
					},
				}, nil)
			},
			wantedARNs: []string{"subnet-1", "subnet-2", "subnet-3"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockAPI := mocks.NewMockapi(ctrl)
			tc.mockEC2Client(mockAPI)

			ec2Client := EC2{
				client: mockAPI,
			}

			arns, err := ec2Client.SubnetIDs(tc.inFilter...)
			if tc.wantedError != nil {
				require.EqualError(t, tc.wantedError, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantedARNs, arns)
			}
		})
	}
}

func TestEC2_SecurityGroups(t *testing.T) {
	testCases := map[string]struct {
		inFilter []Filter

		mockEC2Client func(m *mocks.Mockapi)

		wantedError error
		wantedARNs  []string
	}{
		"failed to get security groups": {
			inFilter: inAppEnvFilters,
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeSecurityGroups(gomock.Any(), &ec2.DescribeSecurityGroupsInput{
					Filters: toEC2Filter(inAppEnvFilters),
				}).Return(nil, errors.New("error getting security groups"))
			},

			wantedError: errors.New("describe security groups: error getting security groups"),
		},
		"get security groups success": {
			inFilter: inAppEnvFilters,
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeSecurityGroups(gomock.Any(), &ec2.DescribeSecurityGroupsInput{
					Filters: toEC2Filter(inAppEnvFilters),
				}).Return(&ec2.DescribeSecurityGroupsOutput{
					SecurityGroups: []types.SecurityGroup{
						{
							GroupId: awsv2.String("sg-1"),
						},
						{
							GroupId: awsv2.String("sg-2"),
						},
					},
				}, nil)
			},

			wantedARNs: []string{"sg-1", "sg-2"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockAPI := mocks.NewMockapi(ctrl)
			tc.mockEC2Client(mockAPI)

			ec2Client := EC2{
				client: mockAPI,
			}

			arns, err := ec2Client.SecurityGroups(inAppEnvFilters...)
			if tc.wantedError != nil {
				require.EqualError(t, tc.wantedError, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantedARNs, arns)
			}
		})
	}
}

func TestEC2_HasDNSSupport(t *testing.T) {
	testCases := map[string]struct {
		vpcID string

		mockEC2Client func(m *mocks.Mockapi)

		wantedError   error
		wantedSupport bool
	}{
		"fail to descibe VPC attribute": {
			vpcID: "mockVPCID",
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeVpcAttribute(gomock.Any(), gomock.Any()).Return(nil, errors.New("some error"))
			},
			wantedError: fmt.Errorf("describe enableDnsSupport attribute for VPC mockVPCID: some error"),
		},
		"success": {
			vpcID: "mockVPCID",
			mockEC2Client: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeVpcAttribute(gomock.Any(), &ec2.DescribeVpcAttributeInput{
					VpcId:     awsv2.String("mockVPCID"),
					Attribute: types.VpcAttributeNameEnableDnsSupport,
				}).Return(&ec2.DescribeVpcAttributeOutput{
					EnableDnsSupport: &types.AttributeBooleanValue{
						Value: awsv2.Bool(true),
					},
				}, nil)
			},
			wantedSupport: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockAPI := mocks.NewMockapi(ctrl)
			tc.mockEC2Client(mockAPI)

			ec2Client := EC2{
				client: mockAPI,
			}

			support, err := ec2Client.HasDNSSupport(tc.vpcID)
			if tc.wantedError != nil {
				require.EqualError(t, tc.wantedError, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantedSupport, support)
			}
		})
	}
}
