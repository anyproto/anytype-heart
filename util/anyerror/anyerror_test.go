package anyerror

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformError(t *testing.T) {
	sep := string(filepath.Separator)

	t.Run("absolute path", func(t *testing.T) {
		pathError := &os.PathError{
			Op:   "read",
			Path: sep + "test" + sep + "file" + sep + "path" + sep,
			Err:  fmt.Errorf("test"),
		}

		resultErrorMessage := "read <masked file path>: test"
		assert.NotNil(t, CleanupError(pathError))
		assert.Equal(t, resultErrorMessage, CleanupError(pathError).Error())
	})

	t.Run("relative path", func(t *testing.T) {
		pathError := &os.PathError{
			Op:   "read",
			Path: "test" + sep + "file",
			Err:  fmt.Errorf("test"),
		}

		resultErrorMessage := "read <masked file path>: test"
		assert.NotNil(t, CleanupError(pathError))
		assert.Equal(t, resultErrorMessage, CleanupError(pathError).Error())
	})

	t.Run("not os path error", func(t *testing.T) {
		err := fmt.Errorf("test")
		resultErrorMessage := "test"
		assert.NotNil(t, CleanupError(err))
		assert.Equal(t, resultErrorMessage, CleanupError(err).Error())
	})

	t.Run("url error", func(t *testing.T) {
		err := &url.Error{URL: "http://test.test", Op: "Test", Err: fmt.Errorf("test")}
		resultErrorMessage := "Test \"<masked url>\": test"
		assert.NotNil(t, CleanupError(err))
		assert.Equal(t, resultErrorMessage, CleanupError(err).Error())
	})
}

func Test_transformBadgerError(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name          string
		pathseparator string
		args          args
		wantErr       error
	}{
		{
			name:          "badger error win",
			pathseparator: "\\",
			args: args{
				err: fmt.Errorf("while opening memtables error: while opening fid: 34 error: while updating skiplist error: while truncate file: C:\\Users\\user1\\AppData\\Roaming\\anytype\\data\\A9xxxxx\\localstore\\000001.sst, error: underlying error\n"),
			},
			wantErr: errors.New("while opening memtables error: while opening fid: 34 error: while updating skiplist error: while truncate file: *\\000001.sst, error: underlying error\n"),
		},
		{
			name:          "badger error mac",
			pathseparator: "/",
			args: args{
				err: fmt.Errorf("while opening memtables error: while opening fid: 34 error: while updating skiplist error: while truncate file: /Users/roman/Library/Application\\ Support/anytype/alpha/data/A9xxxxx/localstore/000002.sst, error: underlying error\n"),
			},
			wantErr: errors.New("while opening memtables error: while opening fid: 34 error: while updating skiplist error: while truncate file: */000002.sst, error: underlying error\n"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.pathseparator != "" && tt.pathseparator != string(os.PathSeparator) {
				t.Skipf("Test is not applicable for the current platform")
			}
			resultErr, _ := anonymizeBadgerError(tt.args.err.Error(), false)

			if tt.wantErr == nil {
				require.Empty(t, resultErr)
				return
			}

			badgerError, _ := anonymizeBadgerError(tt.args.err.Error(), false)
			require.Equal(t, badgerError, tt.wantErr.Error())
		})
	}
}
