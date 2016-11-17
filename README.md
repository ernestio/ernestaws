# ERNESTAWS

master : [![CircleCI](https://circleci.com/gh/ernestio/ernestaws/tree/master.svg?style=svg)](https://circleci.com/gh/ernestio/ernestaws/tree/master) | develop : [![CircleCI](https://circleci.com/gh/ernestio/ernestaws/tree/develop.svg?style=svg)](https://circleci.com/gh/ernestio/ernestaws/tree/develop)

This library aims to be a wrapper on top of aws go sdk, so it concentrates all aws specific logic on ernest.

Example:
```go
package main

import(
  "fmt"

	"github.com/ernestio/ernestaws"
	"github.com/ernestio/ernestaws/network"
)

func main() {
	event := network.New("network.create.aws", "{....}")

	subject, data := ernestaws.Handle(&event)
	fmt.Println("Response: ")
	fmt.Println(subject)
	fmt.Println(data)
}
```

## Using it

You can start by importing


## Contributing

Please read through our
[contributing guidelines](CONTRIBUTING.md).
Included are directions for opening issues, coding standards, and notes on
development.

Moreover, if your pull request contains patches or features, you must include
relevant unit tests.

## Versioning

For transparency into our release cycle and in striving to maintain backward
compatibility, this project is maintained under [the Semantic Versioning guidelines](http://semver.org/).

## Copyright and License

Code and documentation copyright since 2015 r3labs.io authors.

Code released under
[the Mozilla Public License Version 2.0](LICENSE).

