#!/bin/bash

rm -rf pkg/pb/*
mkdir -p pkg/pb
# Generate the protobuf
protoc-wrapper -I/usr/include \
        --proto_path=pkg/proto \
        --go_out=pkg/pb \
        --go_opt=paths=source_relative \
        --go-grpc_out=pkg/pb \
        --go-grpc_opt=paths=source_relative \
        pkg/proto/*.proto
# Generate the swagger.json
protoc-wrapper -I/usr/include \
        --proto_path=pkg/proto \
        --grpc-gateway_out=pkg/pb \
        --grpc-gateway_opt=logtostderr=true \
        --grpc-gateway_opt=paths=source_relative \
        --openapiv2_out=gateway/apidocs \
        --openapiv2_opt=logtostderr=true \
        pkg/proto/service.proto
