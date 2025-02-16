// Copyright 2021 Google LLC
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

package server

import (
	"reflect"
	"strings"
	"testing"

	"github.com/apigee/apigee-remote-service-golib/v2/auth"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestEncodeMetadata(t *testing.T) {
	h := &multitenantContext{
		&Handler{
			orgName:       "org",
			envName:       "*",
			isMultitenant: true,
		},
		"env",
	}
	authContext := &auth.Context{
		Context:        h,
		ClientID:       "clientid",
		AccessToken:    "accesstoken",
		Application:    "application",
		APIProducts:    []string{"prod1", "prod2"},
		DeveloperEmail: "dev@google.com",
		Scopes:         []string{"scope1", "scope2"},
	}
	api := "api"
	metadata, err := encodeAuthMetadata(api, authContext, true)
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{}
	for k, v := range metadata.GetFields() {
		headers[k] = v.GetStringValue()
	}

	equal := func(key, want string) {
		if headers[key] != want {
			t.Errorf("got: '%s', want: '%s'", headers[key], want)
		}
	}

	equal(headerAccessToken, authContext.AccessToken)
	equal(headerAPI, api)
	equal(headerAPIProducts, strings.Join(authContext.APIProducts, ","))
	equal(headerApplication, authContext.Application)
	equal(headerClientID, authContext.ClientID)
	equal(headerDeveloperEmail, authContext.DeveloperEmail)
	equal(headerEnvironment, authContext.Environment())
	equal(headerOrganization, authContext.Organization())
	equal(headerScope, strings.Join(authContext.Scopes, " "))

	api2, ac2 := h.decodeAuthMetadata(metadata.GetFields())
	if api != api2 {
		t.Errorf("got: '%s', want: '%s'", api2, api)
	}

	if !reflect.DeepEqual(*authContext, *ac2) {
		t.Errorf("\ngot:\n%#v,\nwant\n%#v\n", *ac2, *authContext)
	}
}

func TestEncodeMetadataNilCheck(t *testing.T) {
	v, err := encodeAuthMetadata("api", nil, true)
	if err != nil {
		t.Errorf("should not return err: %v", err)
	}
	if v == nil || v.Fields == nil {
		t.Errorf("should not return nil")
	}
}

func TestEncodeMetadataAuthorizedField(t *testing.T) {
	h := &Handler{
		orgName: "org",
		envName: "env",
	}
	authContext := &auth.Context{
		Context:        h,
		ClientID:       "clientid",
		AccessToken:    "accesstoken",
		Application:    "application",
		APIProducts:    []string{"prod1", "prod2"},
		DeveloperEmail: "dev@google.com",
		Scopes:         []string{"scope1", "scope2"},
	}

	metadata, err := encodeAuthMetadata("api", authContext, true)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := metadata.GetFields()[headerAuthorized]
	if !ok {
		t.Fatalf("'x-apigee-authorized' field not found in metadata")
	}
	if value.GetStringValue() != "true" {
		t.Errorf("'x-apigee-authorized' should be true, got %s", value.GetStringValue())
	}

	metadata, err = encodeAuthMetadata("api", authContext, false)
	if err != nil {
		t.Fatal(err)
	}
	_, ok = metadata.GetFields()[headerAuthorized]
	if ok {
		t.Fatalf("should not have 'x-apigee-authorized' field in metadata")
	}
}

func TestEncodeMetadataHeadersExceptions(t *testing.T) {
	h := &Handler{
		orgName: "org",
		envName: "*",
	}
	h.apiHeader = "api"
	metadata := &structpb.Struct{
		Fields: map[string]*structpb.Value{
			headerAPI: structpb.NewStringValue("api"),
		},
	}

	api, ac := h.decodeAuthMetadata(metadata.GetFields())
	if ac.Environment() != "*" {
		t.Errorf("got: %s, want: %s", ac.Environment(), "*")
	}
	if api != "api" {
		t.Errorf("got: %s, want: %s", api, "api")
	}

	h.isMultitenant = true
	api, ac = h.decodeAuthMetadata(metadata.GetFields())
	if api != "api" {
		t.Errorf("got: %s, want: %s", api, "api")
	}
	if ac.Organization() != h.orgName {
		t.Errorf("got: %s, want: %s", ac.Organization(), h.orgName)
	}
	if ac.Environment() != "" {
		t.Errorf("got: %s, want empty string", ac.Environment())
	}

	metadata.Fields[headerEnvironment] = structpb.NewStringValue("test")
	api, ac = h.decodeAuthMetadata(metadata.GetFields())
	if api != "api" {
		t.Errorf("got: %s, want: %s", api, "api")
	}
	if ac.Organization() != h.orgName {
		t.Errorf("got: %s, want: %s", ac.Organization(), h.orgName)
	}
	if ac.Environment() != "test" {
		t.Errorf("got: %s, want: %s", ac.Environment(), "test")
	}
}
