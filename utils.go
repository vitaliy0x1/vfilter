package vfilter

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/repr"
)

func Debug(arg interface{}) {
	if arg != nil {
		repr.Println(arg)
	} else {
		repr.Println("nil")
	}
}

func merge_channels(cs []<-chan Any) <-chan Any {
	var wg sync.WaitGroup
	out := make(chan Any)

	// Start an output goroutine for each input channel in cs.  output
	// copies values from c to out until c is closed, then calls wg.Done.
	output := func(c <-chan Any) {
		for n := range c {
			out <- n
		}
		wg.Done()
	}
	wg.Add(len(cs))
	for _, c := range cs {
		go output(c)
	}

	// Start a goroutine to close out once all the output goroutines are
	// done.  This must start after the wg.Add call.
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// Is the symbol exported by Go? Only names with upper case are exported.
func is_exported(name string) bool {
	switch name {
	// Ignore common methods which should not be exported.
	case "MarshalJSON", "MarshalYAML":
		return false

	default:
		if len(name) == 0 || name[0] == '_' {
			return false
		}

		runes := []rune(name)
		return runes[0] == unicode.ToUpper(runes[0])
	}
}

func _Callable(method_value reflect.Value, field_name string) bool {
	if !method_value.IsValid() {
		return false
	}

	// The name must be exportable.
	if !is_exported(field_name) {
		return false
	}

	// The function must have no args.
	if method_value.Type().NumIn() != 0 {
		return false
	}

	return true
}

func IsNil(a interface{}) bool {
	defer func() { recover() }()
	return a == nil || reflect.ValueOf(a).IsNil()
}

// A real type which encodes to JSON NULL. Using go's nil is dangerous
// because it forces constant checking for nil pointer dereference. It
// is safer to just return this value when VQL needs to return NULL.
type Null struct{}

func (self Null) MarshalJSON() ([]byte, error) {
	return []byte("null"), nil
}

func (self Null) String() string {
	return "Null"
}

func is_null_obj(a Any) bool {
	if a == nil {
		return true
	}

	switch a.(type) {
	case Null, *Null:
		return true
	default:
		return false
	}
}

type _NullAssociative struct{}

func (self _NullAssociative) Applicable(a Any, b Any) bool {
	return is_null_obj(a)
}

func (self _NullAssociative) Associative(scope *Scope, a Any, b Any) (Any, bool) {
	return Null{}, true
}

func (self _NullAssociative) GetMembers(scope *Scope, a Any) []string {
	return []string{}
}

type _NullEqProtocol struct{}

func (self _NullEqProtocol) Applicable(a Any, b Any) bool {
	return is_null_obj(a) || is_null_obj(b)
}

func (self _NullEqProtocol) Eq(scope *Scope, a Any, b Any) bool {
	if is_null_obj(a) && is_null_obj(b) {
		return true
	}
	return false
}

type _NullBoolProtocol struct{}

func (self _NullBoolProtocol) Applicable(a Any) bool {
	return is_null_obj(a)
}

func (self _NullBoolProtocol) Bool(scope *Scope, a Any) bool {
	if is_null_obj(a) {
		return false
	}
	return true
}

func InString(hay *[]string, needle string) bool {
	for _, x := range *hay {
		if x == needle {
			return true
		}
	}

	return false
}

// Returns a unique ID for the object.
func GetID(obj Any) string {
	return fmt.Sprintf("%p", obj)
}

// RowToDict reduces the row into a simple Dict. This materializes any
// lazy queries that are stored in the row into a stable materialized
// dict.
func RowToDict(
	ctx context.Context,
	scope *Scope, row Row) *ordereddict.Dict {

	// Even if it is already a dict we still need to iterate its
	// values to make sure they are fully materialized.
	result := ordereddict.NewDict()
	for _, column := range scope.GetMembers(row) {
		value, pres := scope.Associative(row, column)
		if pres {
			result.Set(column, normalize_value(ctx, scope, value, 0))
		}
	}

	return result
}

func normalize_value(ctx context.Context, scope *Scope, value Any, depth int) Any {
	if depth > 10 {
		return Null{}
	}

	if value == nil {
		value = Null{}
	}

	switch t := value.(type) {
	case Null:
		return value

	case fmt.Stringer:
		return value

	case []byte:
		return string(t)

	case LazyExpr:
		return normalize_value(ctx, scope, t.Reduce(), depth+1)

	case StoredQuery:
		return Materialize(ctx, scope, t)

	default:
		// Pass directly to Json Marshal
		return value
	}
}

// Get a list of similar sounding plugins.
func getSimilarPlugins(scope *Scope, name string) []string {
	result := []string{}
	parts := strings.Split(name, "_")

	scope.Lock()
	defer scope.Unlock()

	for _, part := range parts {
		for k, _ := range scope.plugins {
			if strings.Contains(k, part) && !InString(&result, k) {
				result = append(result, k)
			}
		}
	}

	sort.Strings(result)

	return result
}

func RecoverVQL(scope *Scope) {
	r := recover()
	if r != nil {
		scope.Log("PANIC: %v\n", r)
		buffer := make([]byte, 4096)
		n := runtime.Stack(buffer, false /* all */)
		scope.Log("%s", buffer[:n])
	}
}
