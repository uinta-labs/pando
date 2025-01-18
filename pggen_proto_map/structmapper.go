package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"strings"

	"github.com/fatih/structtag"
	"github.com/iancoleman/strcase"
	"gopkg.in/yaml.v2"
)

func getPackageName(path string) (string, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, path, nil, 0)
	if err != nil {
		return "", err
	}
	if len(pkgs) != 1 {
		return "", fmt.Errorf("expected one package, found %d", len(pkgs))
	}
	for _, pkg := range pkgs {
		return pkg.Name, nil
	}
	return "", fmt.Errorf("no package found")
}

func resolveTypePath(p string) (string, string, error) {
	// github.com/google/uuid.UUID -> github.com/google/uuid, UUID

	parts := strings.Split(p, ".")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid type path: %s", p)
	}

	return strings.Join(parts[:len(parts)-1], "."), parts[len(parts)-1], nil
}

func parseStruct(typePath string) (Struct, error) {
	packagePath, structName, err := resolveTypePath(typePath)
	if err != nil {
		return Struct{}, err
	}

	// Parse the package
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, packagePath, nil, 0)
	if err != nil {
		return Struct{}, err
	}

	if len(pkgs) != 1 {
		return Struct{}, fmt.Errorf("expected one package, found %d", len(pkgs))
	}
	var packageName string
	for _, pkg := range pkgs {
		packageName = pkg.Name
		break
	}

	typeName := fmt.Sprintf("%s.%s", packageName, structName)

	result := Struct{
		Path:     packagePath,
		Name:     structName,
		TypeName: typeName,
	}

	// Find and print struct fields
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				// Find struct type declarations
				ts, ok := n.(*ast.TypeSpec)
				if !ok {
					return true
				}

				// Check if it's the struct we're looking for
				if ts.Name.Name == structName {
					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						return false
					}

					fields := []StructField{}

					for _, field := range st.Fields.List {
						f := StructField{}
						for _, name := range field.Names {
							f.Name = name.Name
						}

						if field.Tag != nil {
							f.Tag = field.Tag.Value
						}

						switch field.Type.(type) {
						case *ast.StarExpr:
							f.Type = fmt.Sprintf("*%s", field.Type.(*ast.StarExpr).X)
						case *ast.ArrayType:
							f.Type = fmt.Sprintf("[]%s", field.Type.(*ast.ArrayType).Elt)
						case ast.Expr:
							f.Type = fmt.Sprintf("%s", field.Type)
						default:
							return false
						}

						fields = append(fields, f)
					}

					result.Fields = fields
				}
				return false
			})
		}
	}

	return result, nil
}

type MappingField struct {
	Name        string `yaml:"name"`
	ConvertFunc string `yaml:"convert_func"`
	Skip        bool   `yaml:"skip"`
}

type MappingItem struct {
	Name   string         `yaml:"name"`
	From   []string       `yaml:"from"`
	Fields []MappingField `yaml:"fields"`
}

type MappingConfig struct {
	PackageName string        `yaml:"package"`
	Version     string        `yaml:"version"`
	Module      string        `yaml:"module"`
	Convert     string        `yaml:"convert"`
	Mappings    []MappingItem `yaml:"mappings"`
}

func (m *MappingConfig) ConvertPackage() (string, string, error) {
	// return "import path", "package name", error
	if m.Convert == "" {
		return "", "", nil
	}

	packageName, err := getPackageName(m.Convert)
	if err != nil {
		return "", "", err
	}

	return m.Convert, packageName, nil
}

func readConfig(path string) (MappingConfig, error) {
	cfg := MappingConfig{}

	contents, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	err = yaml.Unmarshal(contents, &cfg)
	if err != nil {
		return cfg, err
	}

	return cfg, nil
}

type StructField struct {
	Type string
	Name string
	Tag  string
}

type Struct struct {
	Path     string
	TypeName string
	Name     string
	Fields   []StructField
}

func (s *Struct) FindField(target StructField) (StructField, bool) {
	for _, field := range s.Fields {
		if field.Name == target.Name {
			return field, true
		}

		targetTagValue := strings.Replace(target.Tag, "`", "", -1)
		fromTagValue := strings.Replace(field.Tag, "`", "", -1)

		targetTag, err := structtag.Parse(targetTagValue)
		if err != nil {
			log.Panicf("failed to parse tag %s: %s", targetTagValue, err)
		}

		tag, err := structtag.Parse(fromTagValue)
		if err != nil {
			log.Panicf("failed to parse tag %s: %s", fromTagValue, err)
		}

		targetJSONTag, err := targetTag.Get("json")
		if err != nil {
			continue
		}

		jsonTag, err := tag.Get("json")
		if err != nil {
			continue
		}

		if targetJSONTag.Name == jsonTag.Name {
			return field, true
		}
	}
	return StructField{}, false
}

func validGoName(s string) string {
	replacer := strings.NewReplacer("-", "_", ".", "_", "*", "Ptr", "&", "Ref", "{", "", "}", "", " ", "", "[]", "SliceOf", "(", "", ")", "")
	return strcase.ToCamel(replacer.Replace(s))
}

func MapField(from StructField, to StructField, conversionPackage string) (string, bool) {
	if from.Type == to.Type {
		return "from." + from.Name, false
	}

	if strings.HasPrefix(to.Type, "*") && !strings.HasPrefix(from.Type, "*") && strings.TrimPrefix(to.Type, "*") == from.Type {
		return "&from." + from.Name, false
	}
	if strings.HasPrefix(from.Type, "*") && !strings.HasPrefix(to.Type, "*") && strings.TrimPrefix(from.Type, "*") == to.Type {
		return fmt.Sprintf("unwrap(from.%s)", from.Name), false
	}

	if to.Type == "[]byte" && from.Type == "string" {
		return "[]byte(from." + from.Name + ")", false
	}
	if to.Type == "string" && from.Type == "[]byte" {
		return "string(from." + from.Name + ")", false
	}

	convertPrefix := ""
	if conversionPackage != "" {
		convertPrefix = conversionPackage + "."
	}
	return convertPrefix + "Convert" + validGoName(from.Type) + "To" + validGoName(to.Type) + "(from." + from.Name + ")", true
}

func Generate(config MappingConfig) (string, error) {
	b := bytes.NewBuffer(nil)

	conversionsImport, conversionsPackageName, err := config.ConvertPackage()
	if err != nil {
		return "", err
	}

	P := func(strings ...string) {
		for _, s := range strings {
			b.WriteString(s)
		}
		b.WriteString("\n")
	}
	P("package ", config.PackageName)
	P("// Code generated by struct-mapper. DO NOT EDIT.")
	P()
	P("import (")
	P("\t_ \"fmt\"")

	if conversionsImport != "" {
		P("\t\"", config.Module, "/", conversionsImport, "\"")
	}

	mappingImports := make(map[string]bool)

	for _, mapping := range config.Mappings {
		for _, fromTypePath := range mapping.From {
			packagePath, _, err := resolveTypePath(fromTypePath)
			if err != nil {
				return "", err
			}
			mappingImports[packagePath] = true
		}
	}

	for packagePath := range mappingImports {
		P("\t\"", config.Module, "/", packagePath, "\"")
	}

	P(")")
	P()

	for _, mapping := range config.Mappings {
		targetTypePath := mapping.Name

		inspectedTarget, err := parseStruct(targetTypePath)
		if err != nil {
			return "", err
		}

		for _, fromTypePath := range mapping.From {

			inspectedFrom, err := parseStruct(fromTypePath)
			if err != nil {
				return "", err
			}

			P("//goland:noinspection GoSnakeCaseUsage")
			P("func (m *", inspectedTarget.Name, ") From_", strings.Replace(inspectedFrom.TypeName, ".", "_", -1), "(from *", inspectedFrom.TypeName, ") error {")
			P("\tif from == nil {")
			P("\t\treturn fmt.Errorf(\"cannot map nil\")")
			P("\t}")
			P()
			P("\tvar err error")
			P("\tvar _ = err // avoid unused error")
			P()

			for _, targetField := range inspectedTarget.Fields {
				if targetField.Name[0] < 'A' || targetField.Name[0] > 'Z' {
					continue
				}

				fromField, ok := inspectedFrom.FindField(targetField)
				if !ok {
					log.Printf("field %s not found in %s\n", targetField.Name, inspectedFrom.Name)
					continue
				}

				usedCustomDirective := false
				for _, mappingField := range mapping.Fields {
					if mappingField.Skip {
						P("\t// ", targetField.Name, " -> ", fromField.Name)
						P("\t// skipped")
						P()
						usedCustomDirective = true
						continue
					}

					//if mappingField.Name == targetField.Name {
					//	// Mapper if the name of a function that takes the source targetField and returns the target targetField
					//	if mappingField.ConvertFunc != "" {
					//		P("\t// ", targetField.Name, " -> ", conversionsPackageName, ".", mappingField.ConvertFunc, "(from.", fromField.Name, ")")
					//		P("\tm.", targetField.Name, " = ", conversionsPackageName, ".", mappingField.ConvertFunc, "(from.", fromField.Name, ")")
					//		P()
					//		usedCustomDirective = true
					//		continue
					//	}
					//}
				}
				if usedCustomDirective {
					continue
				}

				P("\t// ", targetField.Name, " -> ", fromField.Name)
				conv, returnsError := MapField(fromField, targetField, conversionsPackageName)
				if returnsError {
					P("\tm.", targetField.Name, ", err = ", conv)
					P("\tif err != nil {")
					P("\t\treturn err")
					P("\t}")
				} else {
					P("\tm.", targetField.Name, " = ", conv)
				}
				P()

			}

			P("\treturn nil")
			P("}")
			P()
		}
	}

	return b.String(), nil
}

func GenerateStructMapping() {
	cfg, err := readConfig("../mapping.yaml")
	if err != nil {
		panic(err)
	}

	var outputFilename string
	if len(os.Args) > 1 {
		outputFilename = os.Args[1]
	} else {
		outputFilename = "/tmp/mapper.gen.go"
	}

	output, err := Generate(cfg)
	if err != nil {
		panic(err)
	}

	err = os.WriteFile(outputFilename, []byte(output), 0644)
	if err != nil {
		panic(err)
	}
}
