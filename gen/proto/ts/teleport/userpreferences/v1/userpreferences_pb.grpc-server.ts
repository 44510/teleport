/* eslint-disable */
// @generated by protobuf-ts 2.9.3 with parameter eslint_disable,add_pb_suffix,server_grpc1,ts_nocheck
// @generated from protobuf file "teleport/userpreferences/v1/userpreferences.proto" (package "teleport.userpreferences.v1", syntax proto3)
// tslint:disable
// @ts-nocheck
//
// Copyright 2023 Gravitational, Inc
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
//
import { GetKeyboardLayoutResponse } from "./userpreferences_pb";
import { GetKeyboardLayoutRequest } from "./userpreferences_pb";
import { Empty } from "../../../google/protobuf/empty_pb";
import { UpsertUserPreferencesRequest } from "./userpreferences_pb";
import { GetUserPreferencesResponse } from "./userpreferences_pb";
import { GetUserPreferencesRequest } from "./userpreferences_pb";
import type * as grpc from "@grpc/grpc-js";
/**
 * UserPreferencesService is a service that stores user settings.
 *
 * @generated from protobuf service teleport.userpreferences.v1.UserPreferencesService
 */
export interface IUserPreferencesService extends grpc.UntypedServiceImplementation {
    /**
     * GetUserPreferences returns the user preferences for a given user.
     *
     * @generated from protobuf rpc: GetUserPreferences(teleport.userpreferences.v1.GetUserPreferencesRequest) returns (teleport.userpreferences.v1.GetUserPreferencesResponse);
     */
    getUserPreferences: grpc.handleUnaryCall<GetUserPreferencesRequest, GetUserPreferencesResponse>;
    /**
     * UpsertUserPreferences creates or updates user preferences for a given username.
     *
     * @generated from protobuf rpc: UpsertUserPreferences(teleport.userpreferences.v1.UpsertUserPreferencesRequest) returns (google.protobuf.Empty);
     */
    upsertUserPreferences: grpc.handleUnaryCall<UpsertUserPreferencesRequest, Empty>;
    /**
     * GetUserPreferences returns the user preferences for a given user.
     *
     * @generated from protobuf rpc: GetKeyboardLayout(teleport.userpreferences.v1.GetKeyboardLayoutRequest) returns (teleport.userpreferences.v1.GetKeyboardLayoutResponse);
     */
    getKeyboardLayout: grpc.handleUnaryCall<GetKeyboardLayoutRequest, GetKeyboardLayoutResponse>;
}
/**
 * @grpc/grpc-js definition for the protobuf service teleport.userpreferences.v1.UserPreferencesService.
 *
 * Usage: Implement the interface IUserPreferencesService and add to a grpc server.
 *
 * ```typescript
 * const server = new grpc.Server();
 * const service: IUserPreferencesService = ...
 * server.addService(userPreferencesServiceDefinition, service);
 * ```
 */
export const userPreferencesServiceDefinition: grpc.ServiceDefinition<IUserPreferencesService> = {
    getUserPreferences: {
        path: "/teleport.userpreferences.v1.UserPreferencesService/GetUserPreferences",
        originalName: "GetUserPreferences",
        requestStream: false,
        responseStream: false,
        responseDeserialize: bytes => GetUserPreferencesResponse.fromBinary(bytes),
        requestDeserialize: bytes => GetUserPreferencesRequest.fromBinary(bytes),
        responseSerialize: value => Buffer.from(GetUserPreferencesResponse.toBinary(value)),
        requestSerialize: value => Buffer.from(GetUserPreferencesRequest.toBinary(value))
    },
    upsertUserPreferences: {
        path: "/teleport.userpreferences.v1.UserPreferencesService/UpsertUserPreferences",
        originalName: "UpsertUserPreferences",
        requestStream: false,
        responseStream: false,
        responseDeserialize: bytes => Empty.fromBinary(bytes),
        requestDeserialize: bytes => UpsertUserPreferencesRequest.fromBinary(bytes),
        responseSerialize: value => Buffer.from(Empty.toBinary(value)),
        requestSerialize: value => Buffer.from(UpsertUserPreferencesRequest.toBinary(value))
    },
    getKeyboardLayout: {
        path: "/teleport.userpreferences.v1.UserPreferencesService/GetKeyboardLayout",
        originalName: "GetKeyboardLayout",
        requestStream: false,
        responseStream: false,
        responseDeserialize: bytes => GetKeyboardLayoutResponse.fromBinary(bytes),
        requestDeserialize: bytes => GetKeyboardLayoutRequest.fromBinary(bytes),
        responseSerialize: value => Buffer.from(GetKeyboardLayoutResponse.toBinary(value)),
        requestSerialize: value => Buffer.from(GetKeyboardLayoutRequest.toBinary(value))
    }
};
