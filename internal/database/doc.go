// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package database

//go:generate go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate
//go:generate go run ../cmd/gentables -schema schema.sql -out tables.go
//go:generate go run github.com/elastic/go-licenser@v0.4.2 -license ASL2-Short .
