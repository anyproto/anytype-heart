//go:build integration

package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/anyproto/anytype-heart/util/reflection"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const cacheDir = ".cache"

func cacheFilename(key string) string {
	return filepath.Join(cacheDir, key)
}

func readStringFromCache(key string) (string, error) {
	raw, err := os.ReadFile(cacheFilename(key))
	return string(raw), err
}

func cachedString(key string, rewriteCache bool, proc func() (string, error)) (string, bool, error) {
	result, err := readStringFromCache(key)
	if rewriteCache || os.IsNotExist(err) || result == "" {
		res, err := proc()
		if err != nil {
			return "", false, fmt.Errorf("running proc for caching %s: %w", key, err)
		}
		err = os.WriteFile(cacheFilename(key), []byte(res), 0600)
		if err != nil {
			return "", false, fmt.Errorf("writing cache for %s: %w", key, err)
		}
		return res, false, nil
	}

	return result, true, nil
}

type callCtx struct {
	t     *testing.T
	token string
}

func (c callCtx) newContext() context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "token", c.token)
}

func (s testSession) newCallCtx(t *testing.T) callCtx {
	return callCtx{
		t:     t,
		token: s.token,
	}
}

func call[reqT, respT any](
	cctx callCtx,
	method func(context.Context, reqT, ...grpc.CallOption) (respT, error),
	req reqT,
) respT {
	resp, err := callReturnError(cctx, method, req)
	require.NoError(cctx.t, err)
	require.NotNil(cctx.t, resp)
	return resp
}

func callReturnError[reqT any, respT any](
	cctx callCtx,
	method func(context.Context, reqT, ...grpc.CallOption) (respT, error),
	req reqT,
) (respT, error) {
	name := runtime.FuncForPC(reflect.ValueOf(method).Pointer()).Name()
	name = name[strings.LastIndex(name, ".")+1:]
	name = name[:strings.LastIndex(name, "-")]
	cctx.t.Logf("calling %s", name)

	var nilResp respT

	resp, err := method(cctx.newContext(), req)
	if err != nil {
		return nilResp, err
	}
	_, _, err = reflection.GetError(resp)
	if err != nil {
		return nilResp, err
	}
	require.NotNil(cctx.t, resp)
	return resp, nil
}
