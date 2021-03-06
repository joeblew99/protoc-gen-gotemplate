package main

import (
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/generator"
	"github.com/golang/protobuf/protoc-gen-go/plugin"
	ggdescriptor "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway/descriptor"

	pgghelpers "github.com/moul/protoc-gen-gotemplate/helpers"
)

var (
	registry *ggdescriptor.Registry // some helpers need access to registry
)

func main() {
	g := generator.New()

	data, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		g.Error(err, "reading input")
	}

	if err := proto.Unmarshal(data, g.Request); err != nil {
		g.Error(err, "parsing input proto")
	}

	if len(g.Request.FileToGenerate) == 0 {
		g.Fail("no files to generate")
	}

	g.CommandLineParameters(g.Request.GetParameter())

	// Parse parameters
	var (
		templateDir       = "./templates"
		destinationDir    = "."
		debug             = false
		all               = false
		singlePackageMode = false
	)
	if parameter := g.Request.GetParameter(); parameter != "" {
		for _, param := range strings.Split(parameter, ",") {
			parts := strings.Split(param, "=")
			if len(parts) != 2 {
				log.Printf("Err: invalid parameter: %q", param)
				continue
			}
			switch parts[0] {
			case "template_dir":
				templateDir = parts[1]
				break
			case "destination_dir":
				destinationDir = parts[1]
				break
			case "single-package-mode":
				switch strings.ToLower(parts[1]) {
				case "true", "t":
					singlePackageMode = true
				case "false", "f":
				default:
					log.Printf("Err: invalid value for single-package-mode: %q", parts[1])
				}
				break
			case "debug":
				switch strings.ToLower(parts[1]) {
				case "true", "t":
					debug = true
				case "false", "f":
				default:
					log.Printf("Err: invalid value for debug: %q", parts[1])
				}
				break
			case "all":
				switch strings.ToLower(parts[1]) {
				case "true", "t":
					all = true
				case "false", "f":
				default:
					log.Printf("Err: invalid value for debug: %q", parts[1])
				}
				break
			default:
				log.Printf("Err: unknown parameter: %q", param)
			}
		}
	}

	tmplMap := make(map[string]*plugin_go.CodeGeneratorResponse_File)
	concatOrAppend := func(file *plugin_go.CodeGeneratorResponse_File) {
		if val, ok := tmplMap[file.GetName()]; ok {
			*val.Content += file.GetContent()
		} else {
			tmplMap[file.GetName()] = file
			g.Response.File = append(g.Response.File, file)
		}
	}

	if singlePackageMode {
		registry = ggdescriptor.NewRegistry()
		pgghelpers.SetRegistry(registry)
		if err := registry.Load(g.Request); err != nil {
			g.Error(err, "registry: failed to load the request")
		}
	}

	// Generate the encoders
	for _, file := range g.Request.GetProtoFile() {
		if all {
			if singlePackageMode {
				if _, err := registry.LookupFile(file.GetName()); err != nil {
					g.Error(err, "registry: failed to lookup file %q", file.GetName())
				}
			}
			encoder := NewGenericTemplateBasedEncoder(templateDir, file, debug, destinationDir)
			for _, tmpl := range encoder.Files() {
				concatOrAppend(tmpl)
			}

			continue
		}

		for _, service := range file.GetService() {
			encoder := NewGenericServiceTemplateBasedEncoder(templateDir, service, file, debug, destinationDir)
			for _, tmpl := range encoder.Files() {
				concatOrAppend(tmpl)
			}
		}
	}

	// Generate the protobufs
	g.GenerateAllFiles()

	data, err = proto.Marshal(g.Response)
	if err != nil {
		g.Error(err, "failed to marshal output proto")
	}

	_, err = os.Stdout.Write(data)
	if err != nil {
		g.Error(err, "failed to write output proto")
	}
}
