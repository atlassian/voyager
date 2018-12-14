package crash

import (
	"bytes"
	"fmt"
	"runtime/debug"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

const (
	tailingUnescapedNewLine = "\n"
)

var now = time.Now()

func TestPanicPrint(t *testing.T) {
	t.Parallel()
	var b bytes.Buffer
	stack := debug.Stack()
	newLineEscapedStack := fmt.Sprintf("%+q", stack)
	expected := `{"level":"fatal","time":"` + now.Format(time.RFC3339) + `","msg":"test","stack":` + newLineEscapedStack + `}` + tailingUnescapedNewLine

	logPanicAsJSON(&b, "test", now, stack)

	require.Equal(t, expected, b.String())
}

func TestErrorPrint(t *testing.T) {
	t.Parallel()
	var b bytes.Buffer
	err := errors.New("test")

	stack := fmt.Sprintf("%+q", fmt.Sprintf("%+v", err))

	expected := `{"level":"error","time":"` + now.Format(time.RFC3339) + `","msg":"test","rawmsg":` + stack + `}` + tailingUnescapedNewLine

	logErrorAsJSON(&b, err, now)

	require.Equal(t, expected, b.String())
}
