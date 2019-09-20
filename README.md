# kratos, the client emulator

[![Build Status](https://travis-ci.com/xmidt-org/kratos.svg?branch=master)](https://travis-ci.com/xmidt-org/kratos)
[![codecov.io](http://codecov.io/github/xmidt-org/kratos/coverage.svg?branch=master)](http://codecov.io/github/xmidt-org/kratos?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/xmidt-org/kratos)](https://goreportcard.com/report/github.com/xmidt-org/kratos)

Websocket wrapper that provides a simple API for making new connections,
sending messages on that connection, and providing a way to handle received
messages.

## How to Install
This project uses go modules to manage its dependencies. This is best used with go 1.12+.  to import this module, run:
```
go get github.com/xmidt-org/kratos@latest
```
or add it to your go.mod file for your project.
