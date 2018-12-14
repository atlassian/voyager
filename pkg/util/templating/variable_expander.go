package templating

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/atlassian/voyager/pkg/util"
	"github.com/pkg/errors"
)

var (
	validVariableMatcher = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9\-_.]*$`)
)

// VariableExpander expands variables into the appropriate values
type VariableExpander interface {
	Expand(s string) (interface{}, *util.ErrorList)
	ValidPrefix(s string) bool
}

// VariableResolver resolves variable names to their values
type VariableResolver func(string) (interface{}, error)

type VariableValidator func(string) bool

type variableExpander struct {
	resolver VariableResolver
	prefix   string
}

// NewVariableExpander creates a variable expander that will use the specified resolver
func NewVariableExpander(r VariableResolver, p string) VariableExpander {
	return &variableExpander{
		resolver: r,
		prefix:   p,
	}
}

type stringIter struct {
	src []rune
	pos int
}

func (i *stringIter) next() rune {
	item := i.src[i.pos]
	i.pos++
	return item
}

func (i *stringIter) hasNext() bool {
	return i.pos < len(i.src)
}

func (i *stringIter) peek() rune {
	if !i.hasNext() {
		return 0
	}

	return i.src[i.pos]
}

// The complexity of this method comes in the fact that we want to keep the
// type of the item to add, unless it is being concatenated with an existing item.
// I.e. if src is nil, it means it is not yet being concatenated and we want to
// keep the type
// However, this is only important if the type is not a string, rune or []rune
// as these (string, rune, []rune) all represent strings, so they'll keep
// the "string" type
func concat(src interface{}, itemToAdd interface{}) (interface{}, error) {
	var itemAsRuneList []rune
	switch typedItemToAdd := itemToAdd.(type) {
	case rune:
		itemAsRuneList = []rune{typedItemToAdd}
	case []rune:
		itemAsRuneList = typedItemToAdd
	case string:
		itemAsRuneList = []rune(typedItemToAdd)
	default:
		if src == nil {
			return itemToAdd, nil
		}
		strVal := fmt.Sprintf("%v", itemToAdd)
		itemAsRuneList = []rune(strVal)
	}

	if src == nil {
		return itemAsRuneList, nil
	}

	var srcRuneList []rune
	switch typedVar := src.(type) {
	case string:
		srcRuneList = []rune(typedVar)
	case []rune:
		srcRuneList = typedVar
	default:
		tmpStr := fmt.Sprintf("%v", src)
		srcRuneList = []rune(tmpStr)
	}
	return append(srcRuneList, itemAsRuneList...), nil
}

// Parser for parsing the plain text part of a string
type TextParser struct {
	src       *stringIter
	resolver  VariableResolver
	validator VariableValidator
}

func (sp *TextParser) parse() (interface{}, *util.ErrorList) {
	errorList := util.NewErrorList()
	var result interface{}
	var err error

	for sp.src.hasNext() {
		currChar := sp.src.next()
		if currChar == '$' {
			nextChar := sp.src.peek()
			if nextChar == '$' {
				result, err = concat(result, '$')
				if err != nil {
					errorList.Add(err)
					return nil, errorList
				}

				sp.src.next()
				continue
			}

			if nextChar == '{' {
				sp.src.next()
				varParser := VarParser{src: sp.src, resolver: sp.resolver, validator: sp.validator}
				var parseResult interface{}
				parseResult, err = varParser.parse()
				if err != nil {
					if util.CanRecover(err) {
						errorList.Add(err)
						continue
					} else {
						errorList.Add(err)
						return nil, errorList
					}
				}

				result, err = concat(result, parseResult)
				if err != nil {
					errorList.Add(err)
					return nil, errorList
				}
				continue
			}
		}

		result, err = concat(result, currChar)
		if err != nil {
			errorList.Add(err)
			return nil, errorList
		}
	}

	if errorList.HasErrors() {
		return nil, errorList
	}

	if asRuneSlice, isRune := result.([]rune); isRune {
		return string(asRuneSlice), nil
	}

	return result, nil
}

// Parser for parsing variable references in a string
type VarParser struct {
	src       *stringIter
	resolver  VariableResolver
	validator VariableValidator
}

func (vp *VarParser) parse() (interface{}, error) {
	var varName interface{}
	var err error

	for vp.src.hasNext() {
		currChar := vp.src.next()
		if currChar == '}' {
			varNameString := string(varName.([]rune))
			if !vp.validator(varNameString) {
				return nil, util.NewErrInvalidVariableName(varNameString)
			}
			return vp.resolver(varNameString)
		}
		if currChar == '$' {
			nextChar := vp.src.peek()
			if nextChar == '$' {
				varName, err = concat(varName, '$')
				if err != nil {
					return nil, err
				}

				vp.src.next()
				continue
			}

			if nextChar == '{' {
				vp.src.next()
				varParser := VarParser{src: vp.src, resolver: vp.resolver, validator: vp.validator}
				var parseResult interface{}
				parseResult, err = varParser.parse()
				if err != nil {
					return nil, err
				}

				varName, err = concat(varName, parseResult)
				if err != nil {
					return nil, err
				}
				continue
			}
		}

		varName, err = concat(varName, currChar)
		if err != nil {
			return nil, err
		}

	}

	return nil, errors.New("missing closing bracket")
}

func (ve variableExpander) Expand(s string) (interface{}, *util.ErrorList) {
	iter := stringIter{src: []rune(s)}

	prefixConditionalResolver := func(varName string) (interface{}, error) {
		if strings.HasPrefix(varName, ve.prefix) {
			return ve.resolver(strings.TrimPrefix(varName, ve.prefix))
		}
		return varName, errors.Errorf("required prefix '%s' not found", ve.prefix)
	}

	prefixIgnoringValidator := func(varName string) bool {
		return validVariableMatcher.MatchString(strings.TrimPrefix(varName, ve.prefix))
	}

	parser := TextParser{src: &iter, resolver: prefixConditionalResolver, validator: prefixIgnoringValidator}

	result, err := parser.parse()
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Checks if a given variable has the "correct" prefix
func (ve variableExpander) ValidPrefix(s string) bool {
	iter := stringIter{src: []rune(s)}

	prefixChecker := func(varName string) (interface{}, error) {
		if strings.HasPrefix(varName, ve.prefix) {
			// We return the original variable here but actually don't need the result only any errors that arise (wrong prefixes)
			return varName, nil
		}
		return varName, errors.Errorf("required prefix '%s' not found", ve.prefix)
	}

	prefixIgnoringValidator := func(varName string) bool {
		return validVariableMatcher.MatchString(strings.TrimPrefix(varName, ve.prefix))
	}

	parser := TextParser{src: &iter, resolver: prefixChecker, validator: prefixIgnoringValidator}
	_, errs := parser.parse()
	return errs == nil || !errs.HasErrors() // if parsing had any errors it had an invalid prefix.
}
