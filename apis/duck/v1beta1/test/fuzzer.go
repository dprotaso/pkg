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

package test

import (
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/apis/duck/v1beta1"
	pkgfuzzer "knative.dev/pkg/apis/testing/fuzzer"
	"sigs.k8s.io/randfill"
)

var testConditions = apis.Conditions{{Type: apis.ConditionReady}, {Type: apis.ConditionSucceeded}}

// FuzzerFuncs includes fuzzing funcs for knative.dev/duck v1beta1 Status types
//
// For other examples see
// https://github.com/kubernetes/apimachinery/blob/master/pkg/apis/meta/fuzzer/fuzzer.go
var FuzzerFuncs = fuzzer.MergeFuzzerFuncs(
	func(codecs serializer.CodecFactory) []interface{} {
		return []interface{}{
			func(status *v1beta1.Status, c randfill.Continue) {
				c.FillNoCustom(status) // fuzz the Status
				status.SetConditions(testConditions)
				pkgfuzzer.FuzzConditions(status, c)
			},
		}
	},
)
