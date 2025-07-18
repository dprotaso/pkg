/*
Copyright 2020 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/sync/errgroup"

	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	kubeclient "knative.dev/pkg/client/injection/kube/client/fake"
	"knative.dev/pkg/observability/metrics/metricstest"
)

type fixedConversionController struct {
	path     string
	response *apixv1.ConversionResponse
}

var _ ConversionController = (*fixedConversionController)(nil)

func (fcc *fixedConversionController) Path() string {
	return fcc.path
}

func (fcc *fixedConversionController) Convert(context.Context, *apixv1.ConversionRequest) *apixv1.ConversionResponse {
	return fcc.response
}

func TestConversionEmptyRequestBody(t *testing.T) {
	c := &fixedConversionController{
		path:     "/bazinga",
		response: &apixv1.ConversionResponse{},
	}

	testEmptyRequestBody(t, c)
}

func TestConversionValidResponse(t *testing.T) {
	cc := &fixedConversionController{
		path: "/bazinga",
		response: &apixv1.ConversionResponse{
			UID: types.UID("some-uid"),
			Result: metav1.Status{
				Status: metav1.StatusSuccess,
			},
		},
	}
	test := testSetup(t, withController(cc))

	eg, _ := errgroup.WithContext(test.ctx)
	eg.Go(func() error { return test.webhook.Run(test.ctx.Done()) })
	defer func() {
		test.cancel()
		if err := eg.Wait(); err != nil {
			t.Error("Unable to run controller:", err)
		}
	}()

	if err := waitForServerAvailable(t, test.addr, testTimeout); err != nil {
		t.Fatal("waitForServerAvailable() =", err)
	}
	tlsClient, err := createSecureTLSClient(t, kubeclient.Get(test.ctx), &test.webhook.Options)
	if err != nil {
		t.Fatal("createSecureTLSClient() =", err)
	}

	review := apixv1.ConversionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apiextensions.k8s.io/v1",
			Kind:       "ConversionReview",
		},
		Request: &apixv1.ConversionRequest{
			UID:               types.UID("some-uid"),
			DesiredAPIVersion: "example.com/v1",
			Objects:           []runtime.RawExtension{},
		},
	}

	reqBuf := new(bytes.Buffer)
	err = json.NewEncoder(reqBuf).Encode(&review)
	if err != nil {
		t.Fatal("Failed to marshal conversion review:", err)
	}

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s%s", test.addr, cc.Path()), reqBuf)
	if err != nil {
		t.Fatal("http.NewRequest() =", err)
	}
	req.Header.Add("Content-Type", "application/json")

	response, err := tlsClient.Do(req)
	if err != nil {
		t.Fatal("Failed to get response", err)
	}

	defer response.Body.Close()

	if got, want := response.StatusCode, http.StatusOK; got != want {
		t.Errorf("Response status code = %v, wanted %v", got, want)
	}

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal("Failed to read response body", err)
	}

	reviewResponse := apixv1.ConversionReview{}

	err = json.NewDecoder(bytes.NewReader(responseBody)).Decode(&reviewResponse)
	if err != nil {
		t.Fatal("Failed to decode response:", err)
	}

	if reviewResponse.Response.UID != "some-uid" {
		t.Errorf("expected the response uid to be the stubbed version")
	}

	if diff := cmp.Diff(review.TypeMeta, reviewResponse.TypeMeta); diff != "" {
		t.Errorf("expected the response typeMeta to be the same as the request (-want, +got)\n%s", diff)
	}

	assertConversionMetrics(t, test, cc.response.Result.Status)
}

func TestConversionInvalidResponse(t *testing.T) {
	cc := &fixedConversionController{
		path: "/bazinga",
		response: &apixv1.ConversionResponse{
			UID: types.UID("some-uid"),
			Result: metav1.Status{
				Status: metav1.StatusFailure,
			},
		},
	}
	test := testSetup(t, withController(cc))

	eg, _ := errgroup.WithContext(test.ctx)
	eg.Go(func() error { return test.webhook.Run(test.ctx.Done()) })
	defer func() {
		test.cancel()
		if err := eg.Wait(); err != nil {
			t.Error("Unable to run controller:", err)
		}
	}()

	if err := waitForServerAvailable(t, test.addr, testTimeout); err != nil {
		t.Fatal("waitForServerAvailable() =", err)
	}
	tlsClient, err := createSecureTLSClient(t, kubeclient.Get(test.ctx), &test.webhook.Options)
	if err != nil {
		t.Fatal("createSecureTLSClient() =", err)
	}

	review := apixv1.ConversionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apiextensions.k8s.io/v1",
			Kind:       "ConversionReview",
		},
		Request: &apixv1.ConversionRequest{
			UID:               types.UID("some-uid"),
			DesiredAPIVersion: "example.com/v1",
			Objects:           []runtime.RawExtension{},
		},
	}

	reqBuf := new(bytes.Buffer)
	err = json.NewEncoder(reqBuf).Encode(&review)
	if err != nil {
		t.Fatal("Failed to marshal conversion review:", err)
	}

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s%s", test.addr, cc.Path()), reqBuf)
	if err != nil {
		t.Fatal("http.NewRequest() =", err)
	}
	req.Header.Add("Content-Type", "application/json")

	response, err := tlsClient.Do(req)
	if err != nil {
		t.Fatal("Failed to get response", err)
	}

	defer response.Body.Close()

	if got, want := response.StatusCode, http.StatusOK; got != want {
		t.Errorf("Response status code = %v, wanted %v", got, want)
	}

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal("Failed to read response body", err)
	}

	reviewResponse := apixv1.ConversionReview{}

	err = json.NewDecoder(bytes.NewReader(responseBody)).Decode(&reviewResponse)
	if err != nil {
		t.Fatal("Failed to decode response:", err)
	}

	if reviewResponse.Response.UID != "some-uid" {
		t.Errorf("expected the response uid to be the stubbed version")
	}

	if reviewResponse.Response.Result.Status != metav1.StatusFailure {
		t.Errorf("expected the response uid to be the stubbed version")
	}

	assertConversionMetrics(t, test, cc.response.Result.Status)
}

func assertConversionMetrics(t *testing.T, tc testContext, status string) {
	metricstest.AssertMetrics(t, tc.metricReader,
		metricstest.MetricsPresent(
			otelhttp.ScopeName,
			"http.server.request.body.size",
			"http.server.response.body.size",
			"http.server.request.duration",
		),
		metricstest.MetricsPresent(
			scopeName,
			"kn.webhook.handler.duration",
		),
		metricstest.HasAttributes(
			"", // any scope
			"", // any metric
			WebhookTypeAttr.With(WebhookTypeConversion),
			GroupAttr.With("example.com"),
			VersionAttr.With("v1"),
			StatusAttr.With(strings.ToLower(status)),
		),
	)
}
