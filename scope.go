package vfilter

/* The scope is a common environment passed to all plugins, functions
   and operators.

   The scope contains all the client specific code which velocifilter
   will use to actually execute the query. For example, clients may
   add new plugins (See PluginGeneratorInterface{}), functions (see
   FunctionInterface{}) or various protocol implementations to the
   scope prior to evaluating any queries. This is the main mechanism
   where clients may extend and specialize the VQL language.

   The scope also contains convenience functions allowing clients to
   execute available protocols.

   The scope may be populated with free variables that can be
   referenced by the query.
*/
type Scope struct {
	vars      []Row
	functions map[string]FunctionInterface
	plugins   map[string]PluginGeneratorInterface

	bool        _BoolDispatcher
	eq          _EqDispatcher
	lt          _LtDispatcher
	add         _AddDispatcher
	sub         _SubDispatcher
	mul         _MulDispatcher
	div         _DivDispatcher
	membership  _MembershipDispatcher
	associative _AssociativeDispatcher
	regex       _RegexDispatcher
}

// Tests two values for equality.
func (self *Scope) Eq(a Any, b Any) bool {
	return self.eq.Eq(self, a, b)
}

// Evaluate the truth value of a value.
func (self *Scope) Bool(a Any) bool {
	return self.bool.Bool(self, a)
}

// Is a less than b?
func (self *Scope) Lt(a Any, b Any) bool {
	return self.lt.Lt(self, a, b)
}

// Add a and b together.
func (self *Scope) Add(a Any, b Any) Any {
	return self.add.Add(self, a, b)
}

// Subtract b from a.
func (self *Scope) Sub(a Any, b Any) Any {
	return self.sub.Sub(self, a, b)
}

// Multiply a and b.
func (self *Scope) Mul(a Any, b Any) Any {
	return self.mul.Mul(self, a, b)
}

// Divide b into a.
func (self *Scope) Div(a Any, b Any) Any {
	return self.div.Div(self, a, b)
}

// Is a a member in b?
func (self *Scope) Membership(a Any, b Any) bool {
	return self.membership.Membership(self, a, b)
}

// Get the field member b from a (i.e. a.b).
func (self *Scope) Associative(a Any, b Any) Any {
	return self.associative.Associative(self, a, b)
}

// Does the regex a match object b.
func (self *Scope) Match(a Any, b Any) bool {
	return self.regex.Match(self, a, b)
}

// Add various protocol implementations into this
// scope. Implementations must be one of the supported protocols or
// this function will panic.
func (self *Scope) AddProtocolImpl(implementations ...Any) *Scope {
	for _, imp := range implementations {
		switch t := imp.(type) {
		case BoolProtocol:
			self.bool.AddImpl(t)
		case EqProtocol:
			self.eq.AddImpl(t)
		case LtProtocol:
			self.lt.AddImpl(t)
		case AddProtocol:
			self.add.AddImpl(t)
		case SubProtocol:
			self.sub.AddImpl(t)
		case MulProtocol:
			self.mul.AddImpl(t)
		case DivProtocol:
			self.div.AddImpl(t)
		case MembershipProtocol:
			self.membership.AddImpl(t)
		case AssociativeProtocol:
			self.associative.AddImpl(t)
		case RegexProtocol:
			self.regex.AddImpl(t)
		default:
			Debug(t)
			panic("Unsupported interface")
		}
	}

	return self
}

// Append the variables in Row to the scope.
func (self *Scope) AppendVars(row Row) *Scope {
	result := self

	result.vars = append(result.vars, row)

	return result
}

// Add client function implementations to the scope. Queries using
// this scope can call these functions from within VQL queries.
func (self *Scope) AppendFunctions(functions ...FunctionInterface) *Scope {
	result := self
	for _, function := range functions {
		result.functions[function.Name()] = function
	}

	return result
}

// Add plugins (data sources) to the scope. VQL queries may select
// from these newly added plugins.
func (self *Scope) AppendPlugins(plugins ...PluginGeneratorInterface) *Scope {
	result := self
	for _, plugin := range plugins {
		result.plugins[plugin.Name()] = plugin
	}

	return result
}

// A factory for the default scope. This will add all built in
// protocols for commonly used code. Clients are expected to add their
// own specialized protocols, functions and plugins to specialize
// their scope objects.
func NewScope() *Scope {
	result := Scope{}
	result.functions = make(map[string]FunctionInterface)
	result.plugins = make(map[string]PluginGeneratorInterface)

	// Protocol handlers.
	result.AddProtocolImpl(_BoolImpl{}, _BoolInt{},
		_NumericLt{},
		_StringEq{}, _NumericEq{}, _ArrayEq{}, _DictEq{},
		_AddStrings{}, _AddFloats{},
		_SubFloats{},
		_SubstringMembership{},
		_NumericMul{},
		_NumericDiv{},
		_RowAssociative{},
		_SubstringRegex{},
	)

	// Built in functions.
	return result.AppendFunctions(_DictFunc{}, _SleepPlugin{})
}

// Fetch the field from the scope variables.
func (self *Scope) Resolve(field string) (interface{}, bool) {
	for _, subscope := range self.vars {
		if element, pres := subscope[field]; pres {
			return element, true
		}
	}

	return nil, false
}
