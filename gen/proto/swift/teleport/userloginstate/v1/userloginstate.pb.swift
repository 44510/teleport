// DO NOT EDIT.
// swift-format-ignore-file
//
// Generated by the Swift generator plugin for the protocol buffer compiler.
// Source: teleport/userloginstate/v1/userloginstate.proto
//
// For information on using the generated types, please see the documentation:
//   https://github.com/apple/swift-protobuf/

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

import Foundation
import SwiftProtobuf

// If the compiler emits an error on this type, it is because this file
// was generated by a version of the `protoc` Swift plug-in that is
// incompatible with the version of SwiftProtobuf to which you are linking.
// Please ensure that you are building against the same version of the API
// that was used to generate this file.
fileprivate struct _GeneratedWithProtocGenSwiftVersion: SwiftProtobuf.ProtobufAPIVersionCheck {
  struct _2: SwiftProtobuf.ProtobufAPIVersion_2 {}
  typealias Version = _2
}

/// UserLoginState describes the ephemeral user login state for a user.
struct Teleport_Userloginstate_V1_UserLoginState {
  // SwiftProtobuf.Message conformance is added in an extension below. See the
  // `Message` and `Message+*Additions` files in the SwiftProtobuf library for
  // methods supported on all messages.

  /// header is the header for the resource.
  var header: Teleport_Header_V1_ResourceHeader {
    get {return _header ?? Teleport_Header_V1_ResourceHeader()}
    set {_header = newValue}
  }
  /// Returns true if `header` has been explicitly set.
  var hasHeader: Bool {return self._header != nil}
  /// Clears the value of `header`. Subsequent reads from it will return its default value.
  mutating func clearHeader() {self._header = nil}

  /// spec is the specification for the user login state.
  var spec: Teleport_Userloginstate_V1_Spec {
    get {return _spec ?? Teleport_Userloginstate_V1_Spec()}
    set {_spec = newValue}
  }
  /// Returns true if `spec` has been explicitly set.
  var hasSpec: Bool {return self._spec != nil}
  /// Clears the value of `spec`. Subsequent reads from it will return its default value.
  mutating func clearSpec() {self._spec = nil}

  var unknownFields = SwiftProtobuf.UnknownStorage()

  init() {}

  fileprivate var _header: Teleport_Header_V1_ResourceHeader? = nil
  fileprivate var _spec: Teleport_Userloginstate_V1_Spec? = nil
}

/// Spec is the specification for a user login state.
struct Teleport_Userloginstate_V1_Spec {
  // SwiftProtobuf.Message conformance is added in an extension below. See the
  // `Message` and `Message+*Additions` files in the SwiftProtobuf library for
  // methods supported on all messages.

  /// roles are the user roles attached to the user.
  var roles: [String] = []

  /// traits are the traits attached to the user.
  var traits: [Teleport_Trait_V1_Trait] = []

  /// user_type is the type of user this state represents.
  var userType: String = String()

  /// original_roles are the user roles that are part of the user's static definition. These roles are
  /// not affected by access granted by access lists and are obtained prior to granting access list access.
  var originalRoles: [String] = []

  var unknownFields = SwiftProtobuf.UnknownStorage()

  init() {}
}

#if swift(>=5.5) && canImport(_Concurrency)
extension Teleport_Userloginstate_V1_UserLoginState: @unchecked Sendable {}
extension Teleport_Userloginstate_V1_Spec: @unchecked Sendable {}
#endif  // swift(>=5.5) && canImport(_Concurrency)

// MARK: - Code below here is support for the SwiftProtobuf runtime.

fileprivate let _protobuf_package = "teleport.userloginstate.v1"

extension Teleport_Userloginstate_V1_UserLoginState: SwiftProtobuf.Message, SwiftProtobuf._MessageImplementationBase, SwiftProtobuf._ProtoNameProviding {
  static let protoMessageName: String = _protobuf_package + ".UserLoginState"
  static let _protobuf_nameMap: SwiftProtobuf._NameMap = [
    1: .same(proto: "header"),
    2: .same(proto: "spec"),
  ]

  mutating func decodeMessage<D: SwiftProtobuf.Decoder>(decoder: inout D) throws {
    while let fieldNumber = try decoder.nextFieldNumber() {
      // The use of inline closures is to circumvent an issue where the compiler
      // allocates stack space for every case branch when no optimizations are
      // enabled. https://github.com/apple/swift-protobuf/issues/1034
      switch fieldNumber {
      case 1: try { try decoder.decodeSingularMessageField(value: &self._header) }()
      case 2: try { try decoder.decodeSingularMessageField(value: &self._spec) }()
      default: break
      }
    }
  }

  func traverse<V: SwiftProtobuf.Visitor>(visitor: inout V) throws {
    // The use of inline closures is to circumvent an issue where the compiler
    // allocates stack space for every if/case branch local when no optimizations
    // are enabled. https://github.com/apple/swift-protobuf/issues/1034 and
    // https://github.com/apple/swift-protobuf/issues/1182
    try { if let v = self._header {
      try visitor.visitSingularMessageField(value: v, fieldNumber: 1)
    } }()
    try { if let v = self._spec {
      try visitor.visitSingularMessageField(value: v, fieldNumber: 2)
    } }()
    try unknownFields.traverse(visitor: &visitor)
  }

  static func ==(lhs: Teleport_Userloginstate_V1_UserLoginState, rhs: Teleport_Userloginstate_V1_UserLoginState) -> Bool {
    if lhs._header != rhs._header {return false}
    if lhs._spec != rhs._spec {return false}
    if lhs.unknownFields != rhs.unknownFields {return false}
    return true
  }
}

extension Teleport_Userloginstate_V1_Spec: SwiftProtobuf.Message, SwiftProtobuf._MessageImplementationBase, SwiftProtobuf._ProtoNameProviding {
  static let protoMessageName: String = _protobuf_package + ".Spec"
  static let _protobuf_nameMap: SwiftProtobuf._NameMap = [
    1: .same(proto: "roles"),
    2: .same(proto: "traits"),
    3: .standard(proto: "user_type"),
    4: .standard(proto: "original_roles"),
  ]

  mutating func decodeMessage<D: SwiftProtobuf.Decoder>(decoder: inout D) throws {
    while let fieldNumber = try decoder.nextFieldNumber() {
      // The use of inline closures is to circumvent an issue where the compiler
      // allocates stack space for every case branch when no optimizations are
      // enabled. https://github.com/apple/swift-protobuf/issues/1034
      switch fieldNumber {
      case 1: try { try decoder.decodeRepeatedStringField(value: &self.roles) }()
      case 2: try { try decoder.decodeRepeatedMessageField(value: &self.traits) }()
      case 3: try { try decoder.decodeSingularStringField(value: &self.userType) }()
      case 4: try { try decoder.decodeRepeatedStringField(value: &self.originalRoles) }()
      default: break
      }
    }
  }

  func traverse<V: SwiftProtobuf.Visitor>(visitor: inout V) throws {
    if !self.roles.isEmpty {
      try visitor.visitRepeatedStringField(value: self.roles, fieldNumber: 1)
    }
    if !self.traits.isEmpty {
      try visitor.visitRepeatedMessageField(value: self.traits, fieldNumber: 2)
    }
    if !self.userType.isEmpty {
      try visitor.visitSingularStringField(value: self.userType, fieldNumber: 3)
    }
    if !self.originalRoles.isEmpty {
      try visitor.visitRepeatedStringField(value: self.originalRoles, fieldNumber: 4)
    }
    try unknownFields.traverse(visitor: &visitor)
  }

  static func ==(lhs: Teleport_Userloginstate_V1_Spec, rhs: Teleport_Userloginstate_V1_Spec) -> Bool {
    if lhs.roles != rhs.roles {return false}
    if lhs.traits != rhs.traits {return false}
    if lhs.userType != rhs.userType {return false}
    if lhs.originalRoles != rhs.originalRoles {return false}
    if lhs.unknownFields != rhs.unknownFields {return false}
    return true
  }
}
