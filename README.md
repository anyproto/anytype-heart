### Anytype Middleware Library
[![CircleCI](https://circleci.com/gh/anytypeio/go-anytype-middleware/tree/master.svg?style=svg&circle-token=eb74d38301ec933d25eb6778f662c94b175186ef)](https://circleci.com/gh/anytypeio/go-anytype-middleware/tree/master)

#### How to build

1. Install Golang 1.12.* [from here](http://golang.org/dl/)
2. `make setup` to install deps
3. `make build-lib` to build C(`.so`) library into `dist` folder
4. `make build-js` to build NodeJS Addon into `jsaddon/build`
5. `npm install & npm build:ts` to compile proto files for TS/JS to `build/ts`

#### Run tests
GO test:
```
make test
```

NodeJS addon test:
```
cd jsaddon
npm run test
```
