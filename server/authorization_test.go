// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package protostruct supports operations on the protocol buffer Struct message.
package server

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/apigee/apigee-remote-service-golib/auth"
	libAuth "github.com/apigee/apigee-remote-service-golib/auth"
	apigeeContext "github.com/apigee/apigee-remote-service-golib/context"
	"github.com/apigee/apigee-remote-service-golib/product"
	"github.com/apigee/apigee-remote-service-golib/quota"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	v2 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v2"
	"github.com/gogo/googleapis/google/rpc"
	pb "github.com/golang/protobuf/ptypes/struct"
)

func TestCheck(t *testing.T) {

	jwtClaims := &pb.Struct{
		Fields: map[string]*pb.Value{
			"apigee": {
				Kind: &pb.Value_StructValue{
					StructValue: &pb.Struct{
						Fields: map[string]*pb.Value{
							"api_product_list": {
								Kind: &pb.Value_StringValue{
									StringValue: "product1,product2",
								},
							},
						},
					},
				},
			},
		},
	}

	headers := map[string]string{
		headerClientID: "clientID",
	}

	products := product.ProductsNameMap{
		"product1": &product.APIProduct{
			DisplayName: "product1",
		},
	}

	uri := "path?x-api-key=foo"
	req := &v2.CheckRequest{
		Attributes: &v2.AttributeContext{
			Request: &v2.AttributeContext_Request{
				Http: &v2.AttributeContext_HttpRequest{
					Path:    uri,
					Headers: headers,
				},
			},
			MetadataContext: &core.Metadata{
				FilterMetadata: map[string]*pb.Struct{
					jwtFilterMetadataKey: jwtClaims,
				},
			},
		},
	}
	req.Attributes.GetMetadataContext()

	testAuthMan := &testAuthMan{}
	testProductMan := &testProductMan{
		resolve: true,
	}
	testQuotaMan := &testQuotaMan{}
	server := AuthorizationServer{
		handler: &Handler{
			rejectUnauthorized: true,
			apiKeyClaim:        headerClientID,
			targetHeader:       headerAPI,
			apiKeyHeader:       "x-api-key",
			authMan:            testAuthMan,
			productMan:         testProductMan,
			quotaMan:           testQuotaMan,
			jwtProviderKey:     "apigee",
		},
	}

	// no target header
	var resp *v2.CheckResponse
	var err error
	if resp, err = server.Check(context.Background(), req); err != nil {
		t.Errorf("should not get error. got: %s", err)
	}
	if resp.Status.Code != int32(rpc.UNAUTHENTICATED) {
		t.Errorf("got: %d, want: %d", resp.Status.Code, int32(rpc.UNAUTHENTICATED))
	}
	headers[headerAPI] = "api"

	// ErrNoAuth
	testAuthMan.sendError = libAuth.ErrNoAuth
	if resp, err = server.Check(context.Background(), req); err != nil {
		t.Errorf("should not get error. got: %s", err)
	}
	if resp.Status.Code != int32(rpc.UNAUTHENTICATED) {
		t.Errorf("got: %d, want: %d", resp.Status.Code, int32(rpc.UNAUTHENTICATED))
	}

	// ErrBadAuth
	testAuthMan.sendError = libAuth.ErrBadAuth
	if resp, err = server.Check(context.Background(), req); err != nil {
		t.Errorf("should not get error. got: %s", err)
	}
	if resp.Status.Code != int32(rpc.PERMISSION_DENIED) {
		t.Errorf("got: %d, want: %d", resp.Status.Code, int32(rpc.PERMISSION_DENIED))
	}

	// ErrInternalError
	testAuthMan.sendError = libAuth.ErrInternalError
	if resp, err = server.Check(context.Background(), req); err != nil {
		t.Errorf("should not get error. got: %s", err)
	}
	if resp.Status.Code != int32(rpc.INTERNAL) {
		t.Errorf("got: %d, want: %d", resp.Status.Code, int32(rpc.INTERNAL))
	}

	// reset auth error
	testAuthMan.sendError = nil

	// no products
	if resp, err = server.Check(context.Background(), req); err != nil {
		t.Errorf("should not get error. got: %s", err)
	}
	if resp.Status.Code != int32(rpc.PERMISSION_DENIED) {
		t.Errorf("got: %d, want: %d", resp.Status.Code, int32(rpc.PERMISSION_DENIED))
	}

	// no matched products
	testProductMan.products = products
	testProductMan.resolve = false
	if resp, err = server.Check(context.Background(), req); err != nil {
		t.Errorf("should not get error. got: %s", err)
	}
	if resp.Status.Code != int32(rpc.PERMISSION_DENIED) {
		t.Errorf("got: %d, want: %d", resp.Status.Code, int32(rpc.PERMISSION_DENIED))
	}
	testProductMan.resolve = true

	// quota exceeded
	products["product1"].QuotaLimitInt = 2
	testQuotaMan.exceeded = 2
	if resp, err = server.Check(context.Background(), req); err != nil {
		t.Errorf("should not get error. got: %s", err)
	}
	if resp.Status.Code != int32(rpc.RESOURCE_EXHAUSTED) {
		t.Errorf("got: %d, want: %d", resp.Status.Code, int32(rpc.RESOURCE_EXHAUSTED))
	}
	code := resp.HttpResponse.(*v2.CheckResponse_DeniedResponse).DeniedResponse.Status.Code
	if code != http.StatusTooManyRequests {
		t.Errorf("got: %d, want: %d", code, http.StatusTooManyRequests)
	}
	testQuotaMan.exceeded = 0

	// quota error
	testQuotaMan.sendError = errors.New("quota error")
	if resp, err = server.Check(context.Background(), req); err != nil {
		t.Errorf("should not get error. got: %s", err)
	}
	if resp.Status.Code != int32(rpc.INTERNAL) {
		t.Errorf("got: %d, want: %d", resp.Status.Code, int32(rpc.INTERNAL))
	}
	testQuotaMan.sendError = nil

	// all OK
	if resp, err = server.Check(context.Background(), req); err != nil {
		t.Errorf("should not get error. got: %s", err)
	}
	if resp.Status.Code != int32(rpc.OK) {
		t.Errorf("got: %d, want: %d", resp.Status.Code, int32(rpc.OK))
	}

	// testAuthMan.ctx
	if testAuthMan.apiKey != "foo" {
		t.Errorf("got: %s, want: %s", testAuthMan.apiKey, "foo")
	}
	// testAuthMan.claims
	if testAuthMan.apiKeyClaimKey != headerClientID {
		t.Errorf("got: %s, want: %s", testAuthMan.apiKeyClaimKey, headerClientID)
	}

	// non-existing jwtProviderKey
	server.handler.jwtProviderKey = "not-apigee"
	if resp, err = server.Check(context.Background(), req); err != nil {
		t.Errorf("should not get error. got: %s", err)
	}
	if resp.Status.Code != int32(rpc.OK) {
		t.Errorf("got: %d, want: %d", resp.Status.Code, int32(rpc.OK))
	}

	// testAuthMan.claims should be nil
	if len(testAuthMan.claims) != 0 {
		t.Errorf("got: %d, want: empty claims", len(testAuthMan.claims))
	}

	// empty jwtProviderKey to enter the claims loop
	server.handler.jwtProviderKey = ""
	if resp, err = server.Check(context.Background(), req); err != nil {
		t.Errorf("should not get error. got: %s", err)
	}
	if resp.Status.Code != int32(rpc.OK) {
		t.Errorf("got: %d, want: %d", resp.Status.Code, int32(rpc.OK))
	}

	// testAuthMan.claims should be nil
	if len(testAuthMan.claims) != 1 {
		t.Errorf("got: %d, want: claims length to be 1", len(testAuthMan.claims))
	}

	// check deny when rejectUnauthorized = false
	server.handler.rejectUnauthorized = false
	testProductMan.resolve = false
	if resp, err = server.Check(context.Background(), req); err != nil {
		t.Errorf("should not get error. got: %s", err)
	}
	if resp.Status.Code != int32(rpc.OK) {
		t.Errorf("got: %d, want: %d", resp.Status.Code, int32(rpc.OK))
	}
}

type testAuthMan struct {
	sendError      error
	ctx            apigeeContext.Context
	apiKey         string
	claims         map[string]interface{}
	apiKeyClaimKey string
}

func (a *testAuthMan) Close() {}
func (a *testAuthMan) Authenticate(ctx apigeeContext.Context, apiKey string, claims map[string]interface{},
	apiKeyClaimKey string) (*auth.Context, error) {
	a.ctx = ctx
	a.apiKey = apiKey
	a.claims = claims
	a.apiKeyClaimKey = apiKeyClaimKey

	if a.sendError != nil {
		return nil, a.sendError
	}

	authContext := &auth.Context{
		Context:     ctx,
		APIProducts: []string{"product 1"},
	}
	return authContext, nil
}

type testProductMan struct {
	products map[string]*product.APIProduct
	resolve  bool
}

func (p *testProductMan) Close() {}
func (p *testProductMan) Products() product.ProductsNameMap {
	return p.products
}
func (p *testProductMan) Authorize(ac *auth.Context, target, path, method string) []product.AuthorizedOperation {
	if !p.resolve {
		return nil
	}
	values := []product.AuthorizedOperation{}
	for _, p := range p.products {
		values = append(values, product.AuthorizedOperation{
			ID:         p.DisplayName,
			QuotaLimit: 42,
		})
	}
	return values
}

type testQuotaMan struct {
	exceeded  int64
	sendError error
}

func (q *testQuotaMan) Start() {}
func (q *testQuotaMan) Close() {}
func (q *testQuotaMan) Apply(auth *auth.Context, p product.AuthorizedOperation, args quota.Args) (*quota.Result, error) {
	if q.sendError != nil {
		return nil, q.sendError
	}
	return &quota.Result{
		Exceeded: q.exceeded,
	}, nil
}
