# kratos, the client emulator

[![Build Status](https://travis-ci.org/Comcast/kratos.svg?branch=master)](https://travis-ci.org/Comcast/kratos)
[![codecov.io](http://codecov.io/github/Comcast/kratos/coverage.svg?branch=master)](http://codecov.io/github/Comcast/kratos?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/Comcast/kratos)](https://goreportcard.com/report/github.com/Comcast/kratos)

Websocket wrapper that provides a simple API for making new connections,
sending messages on that connection, and providing a way to handle received
messages.

## How to Install
This project uses [glide](https://glide.sh) to manage its dependencies. To install, first download `glide` at the provided link, and then
create a `glide.yaml` in `$GOPATH/src/myProject` (where `myProject` is the project that you're using `kratos` with). Running `glide install`
with a properly formatted `glide.yaml` should pull down `kratos` and then all the dependencies that `kratos` has.

## Sample `glide.yaml` to Build `kratos/example/main.go`
Below is a sample `glide.yaml` that you can use in conjunction with the instructions (located further down in this README).
```
package: myProject
import:
- package: github.com/Comcast/kratos
```

## Instructions for Building Sample `main.go` File:

- make sure that you have [golang](https://golang.org) installed and running
- make sure that you have [glide](https://glide.sh) installed
- change directories in your computer to the directory that you want `kratos` to live in (henceforth called `<root>`)
- we’ll call the “project” we’re creating to run the `kratos` sample file `myProject`

```
$ #create the directories for myProject from <root>
$ mkdir -p myProject/src/myProject
$
$ # move in to the top level of the newly created directory and
$ # set your $GOPATH to point there (this is for dependencies and such)
$ cd <root>/myProject
$ export GOPATH=`pwd`
$
$ # put the provided glide.yaml file (shown above),
$ # in the directory <root>/myProject/src/myProject
$
$ # run `glide install` from <root>/myProject/src/myProject
$ # (will only work if you put the `glide.yaml` file in this directory)
$ glide install
$
$ # copy the main.go file included in kratos into <root>/myProject/src/myProject
$ cd <root>/myProject/src/myProject
$ cp <root>/myProject/src/myProject/vendor/github.com/Comcast/kratos/example/main.go .
$
$ # run `glide up` so we can grab the stuff that `main.go` says it needs
$ # these weren't grabbed initially by glide since glide didn't have any import
$ # statements to look at within code
$ cd <root>/myProject/src/myProject
$ glide up
$
$ # change directories to <root>/myProject/src/myProject and build myProject
$ cd <root>/myProject/src/myProject
$ go build
$
$ # run the example file
$ ./myProject
```

