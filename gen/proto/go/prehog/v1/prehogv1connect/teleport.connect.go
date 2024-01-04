// Copyright 2023 Gravitational, Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by protoc-gen-connect-go. DO NOT EDIT.
//
// Source: prehog/v1/teleport.proto

package prehogv1connect

import (
	context "context"
	errors "errors"
	connect_go "github.com/bufbuild/connect-go"
	v1 "github.com/gravitational/teleport/gen/proto/go/prehog/v1"
	http "net/http"
	strings "strings"
)

// This is a compile-time assertion to ensure that this generated file and the connect package are
// compatible. If you get a compiler error that this constant is not defined, this code was
// generated with a version of connect newer than the one compiled into your binary. You can fix the
// problem by either regenerating this code with an older version of connect or updating the connect
// version compiled into your binary.
const _ = connect_go.IsAtLeastVersion0_1_0

const (
	// TeleportReportingServiceName is the fully-qualified name of the TeleportReportingService service.
	TeleportReportingServiceName = "prehog.v1.TeleportReportingService"
)

// These constants are the fully-qualified names of the RPCs defined in this package. They're
// exposed at runtime as Spec.Procedure and as the final two segments of the HTTP route.
//
// Note that these are different from the fully-qualified method names used by
// google.golang.org/protobuf/reflect/protoreflect. To convert from these constants to
// reflection-formatted method names, remove the leading slash and convert the remaining slash to a
// period.
const (
	// TeleportReportingServiceSubmitUsageReportsProcedure is the fully-qualified name of the
	// TeleportReportingService's SubmitUsageReports RPC.
	TeleportReportingServiceSubmitUsageReportsProcedure = "/prehog.v1.TeleportReportingService/SubmitUsageReports"
)

// TeleportReportingServiceClient is a client for the prehog.v1.TeleportReportingService service.
type TeleportReportingServiceClient interface {
	// encodes and forwards usage reports to the PostHog event database; each
	// event is annotated with some properties that depend on the identity of the
	// caller:
	//   - tp.account_id (UUID in string form, can be empty if missing from the
	//     license)
	//   - tp.license_name (should always be a UUID)
	//   - tp.license_authority (name of the authority that signed the license file
	//     used for authentication)
	//   - tp.is_cloud (boolean)
	SubmitUsageReports(context.Context, *connect_go.Request[v1.SubmitUsageReportsRequest]) (*connect_go.Response[v1.SubmitUsageReportsResponse], error)
}

// NewTeleportReportingServiceClient constructs a client for the prehog.v1.TeleportReportingService
// service. By default, it uses the Connect protocol with the binary Protobuf Codec, asks for
// gzipped responses, and sends uncompressed requests. To use the gRPC or gRPC-Web protocols, supply
// the connect.WithGRPC() or connect.WithGRPCWeb() options.
//
// The URL supplied here should be the base URL for the Connect or gRPC server (for example,
// http://api.acme.com or https://acme.com/grpc).
func NewTeleportReportingServiceClient(httpClient connect_go.HTTPClient, baseURL string, opts ...connect_go.ClientOption) TeleportReportingServiceClient {
	baseURL = strings.TrimRight(baseURL, "/")
	return &teleportReportingServiceClient{
		submitUsageReports: connect_go.NewClient[v1.SubmitUsageReportsRequest, v1.SubmitUsageReportsResponse](
			httpClient,
			baseURL+TeleportReportingServiceSubmitUsageReportsProcedure,
			opts...,
		),
	}
}

// teleportReportingServiceClient implements TeleportReportingServiceClient.
type teleportReportingServiceClient struct {
	submitUsageReports *connect_go.Client[v1.SubmitUsageReportsRequest, v1.SubmitUsageReportsResponse]
}

// SubmitUsageReports calls prehog.v1.TeleportReportingService.SubmitUsageReports.
func (c *teleportReportingServiceClient) SubmitUsageReports(ctx context.Context, req *connect_go.Request[v1.SubmitUsageReportsRequest]) (*connect_go.Response[v1.SubmitUsageReportsResponse], error) {
	return c.submitUsageReports.CallUnary(ctx, req)
}

// TeleportReportingServiceHandler is an implementation of the prehog.v1.TeleportReportingService
// service.
type TeleportReportingServiceHandler interface {
	// encodes and forwards usage reports to the PostHog event database; each
	// event is annotated with some properties that depend on the identity of the
	// caller:
	//   - tp.account_id (UUID in string form, can be empty if missing from the
	//     license)
	//   - tp.license_name (should always be a UUID)
	//   - tp.license_authority (name of the authority that signed the license file
	//     used for authentication)
	//   - tp.is_cloud (boolean)
	SubmitUsageReports(context.Context, *connect_go.Request[v1.SubmitUsageReportsRequest]) (*connect_go.Response[v1.SubmitUsageReportsResponse], error)
}

// NewTeleportReportingServiceHandler builds an HTTP handler from the service implementation. It
// returns the path on which to mount the handler and the handler itself.
//
// By default, handlers support the Connect, gRPC, and gRPC-Web protocols with the binary Protobuf
// and JSON codecs. They also support gzip compression.
func NewTeleportReportingServiceHandler(svc TeleportReportingServiceHandler, opts ...connect_go.HandlerOption) (string, http.Handler) {
	teleportReportingServiceSubmitUsageReportsHandler := connect_go.NewUnaryHandler(
		TeleportReportingServiceSubmitUsageReportsProcedure,
		svc.SubmitUsageReports,
		opts...,
	)
	return "/prehog.v1.TeleportReportingService/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case TeleportReportingServiceSubmitUsageReportsProcedure:
			teleportReportingServiceSubmitUsageReportsHandler.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

// UnimplementedTeleportReportingServiceHandler returns CodeUnimplemented from all methods.
type UnimplementedTeleportReportingServiceHandler struct{}

func (UnimplementedTeleportReportingServiceHandler) SubmitUsageReports(context.Context, *connect_go.Request[v1.SubmitUsageReportsRequest]) (*connect_go.Response[v1.SubmitUsageReportsResponse], error) {
	return nil, connect_go.NewError(connect_go.CodeUnimplemented, errors.New("prehog.v1.TeleportReportingService.SubmitUsageReports is not implemented"))
}
