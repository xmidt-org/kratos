# kratos, the client emulator

[![Build Status](https://travis-ci.org/Comcast/kratos.svg?branch=master)](https://travis-ci.org/Comcast/kratos)
[![codecov.io](http://codecov.io/github/Comcast/kratos/coverage.svg?branch=master)](http://codecov.io/github/Comcast/kratos?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/Comcast/kratos)](https://goreportcard.com/report/github.com/Comcast/kratos)

Websocket wrapper that provides a simple API for making new connections,
sending messages on that connection, and providing a way to handle received
messages.

## `Canticle` file that will fetch dependencies (steps outlined below):
```
[
    {
        "Root": "github.com/Comcast/webpa-common",
        "Revision": "779f8a161755c8cc0008e62687344f3c0ab1d47c"
    },
    {
        "Root": "github.com/gorilla/websocket",
        "Revision": "e8f0f8aaa98dfb6586cbdf2978d511e3199a960a"
    },
    {
        "Root": "github.com/nu7hatch/gouuid",
        "Revision": "179d4d0c4d8d407a32af483c2354df1d2c91e6c3"
    },
    {
        "Root": "github.com/ugorji/go",
        "Revision": "faddd6128c66c4708f45fdc007f575f75e592a3c"
    }
]
```

## Instructions for building sample `main.go` file:

- make sure that you have golang installed and running (link: https://golang.org)
- make sure that you have Canticle installed (link: http://canticle.io)
- if you don't have a `$GOBIN` path, make one and put the executable that comes with Canticle in it
- ensure that your `$GOBIN` is included in your `$PATH`
- change directories in your computer to the directory that you want `kratos` to live in (henceforth called `<root>`)
- we’ll call the “project” we’re creating to run the `kratos` sample file `mytest`

```
$ #create the directories for mytest from <root>
$ mkdir -p mytest/src/mytest
$
$ # move in to the top level of the newly created directory and
$ # set your $GOPATH to point there (this is for dependencies and such)
$ cd <root>/mytest
$ export GOPATH=`pwd`
$
$ # change directories into the src directory and make the location for kratos
$ cd <root>/mytest/src
$ mkdir -p github.com/Comcast
$
$ # change directories into the one you just created and clone kratos into it
$ cd <root>/mytest/src/github.com/Comcast/
$ git clone "https://github.com/comcast/kratos"
$
$ # copy the main.go file included in kratos into <root>/mytest/src/mytest
$ cd <root>/mytest/src/mytest
$ cp <root>/mytest/src/github.com/Comcast/kratos/example/main.go .
$
$ # create a Canticle file (not demonstrated here) and copy it to <root>/mytest/src
$ cp <path_to_Canticle_file>/Canticle <root>/mytest/src
$
$ # change directories to <root>/mytest/src and run `cant get -v`
$ # (cant is the binary file included when installing Canticle and
$ # must be somewhere in your $GOBIN, which you include in your $PATH)
$ cd <root>/mytest/src
$ cant get -v
$
$ # change directories to <root>/mytest/src/mytest and build mytest
$ cd <root>/mytest/src/mytest
$ go build
$
$ # run the example file
$ ./mytest
```

