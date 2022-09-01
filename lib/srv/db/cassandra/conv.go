/*
Copyright 2022 Gravitational, Inc.

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

package cassandra

import (
	"fmt"

	"github.com/datastax/go-cassandra-native-protocol/message"
	"github.com/datastax/go-cassandra-native-protocol/primitive"

	"github.com/gravitational/teleport/api/types/events"
)

func eventTypesToString(ets []primitive.EventType) []string {
	out := make([]string, 0, len(ets))
	for _, v := range ets {
		out = append(out, string(v))
	}
	return out
}

func batchChildToProto(batches []*message.BatchChild) []*events.CassandraBatch_BatchChild {
	out := make([]*events.CassandraBatch_BatchChild, 0, len(batches))
	for _, v := range batches {
		out = append(out, &events.CassandraBatch_BatchChild{
			QueryOrId: fmt.Sprintf("%+v", v.Id),
			Values:    convBatchChildValues(v.Values),
		})
	}
	return out
}

func convBatchChildValues(values []*primitive.Value) []*events.CassandraBatch_BatchChild_Value {
	out := make([]*events.CassandraBatch_BatchChild_Value, 0, len(values))
	for _, v := range values {
		out = append(out, &events.CassandraBatch_BatchChild_Value{
			Type:     uint32(v.Type),
			Contents: fmt.Sprintf("%+v", v.Contents),
		})

	}
	return out
}
