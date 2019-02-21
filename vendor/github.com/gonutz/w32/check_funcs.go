//+build ignore

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"strings"
)

type walker struct {
	call string
	ok   bool
}

func (w *walker) Visit(node ast.Node) ast.Visitor {
	if call, ok := node.(*ast.CallExpr); ok {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if sel.Sel.Name == "Call" {
				if id, ok := sel.X.(*ast.Ident); ok {
					if id.Name == w.call {
						w.ok = true
						return nil
					}
				}
			}
		}
	}
	return w
}

func main() {
	funcs, err := ioutil.ReadFile("functions.go")
	check(err)
	var fs token.FileSet
	astFile, err := parser.ParseFile(&fs, "", funcs, 0)
	check(err)
	for _, decl := range astFile.Decls {
		if f, ok := decl.(*ast.FuncDecl); ok {
			if ast.IsExported(f.Name.Name) {
				upperName := f.Name.Name
				lowerName := strings.ToLower(upperName[0:1]) + upperName[1:]
				w := walker{call: lowerName}
				ast.Walk(&w, f.Body)
				if !w.ok {
					fmt.Println(upperName)
				}
			}
		}
	}

	//if dummyFunc, ok := astFile.Decls[0].(*ast.FuncDecl); ok {
	//	statements = dummyFunc.Body.List
	//	offset = func(pos token.Pos) int {
	//		return int(pos) - len(pre)
	//	}
	//} else {
	//	err = errors.New("dummy func is not a func")
	//}
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
