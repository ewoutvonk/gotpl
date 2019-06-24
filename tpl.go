package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"text/template"
	"gopkg.in/yaml.v2"
	"github.com/kubernetes/helm/pkg/strvals"
	"github.com/Masterminds/sprig"
	"path"
	"path/filepath"
	"strings"
	docopt "github.com/docopt/docopt-go"
	"k8s.io/helm/pkg/chartutil"
	"github.com/imdario/mergo"
)

/* 
	I've taken code from:
		https://github.com/fpytloun/gotpl/blob/423feb83aaa7ce29f2034b3e2d5098e1f926355d/Dockerfile (Dockerfile)
		https://github.com/mfriedenhagen/gotpl/blob/9b0d30133af3e3576de1f918409953f03e68a0a1/tpl.go (sprig)
		https://github.com/svenbs/gotpl/blob/6c941be6f62a4016923583c5fc518294fc03d5f5/tpl.go (getEnvironment)
		https://github.com/mcandre/gotpl/blob/9f589b0575eb90022f4025845b30d24706a7e783/tpl.go (flag_values)
	TODO:
		https://github.com/ctchurch/gotpl/blob/c3d531f8f28e8b69a7ab91c6e9c96ef0db1d0287/tpl.go
		https://github.com/CDKGlobal/gotpl/blob/eb78a845f939bbaed850fd172627da7adfcd7735/tpl.go
		https://github.com/PiotrTrzpil/gotpl/blob/a96c01011375d1e10fd7fab5083d3919f903b618/tpl.go
*/

/*
	Related documentation:
		https://github.com/docopt/docopt#help-message-format
		https://helm.sh/docs/helm/#helm-install
		https://golang.org/pkg/strings/#Split
*/

func FuncMap() template.FuncMap {
	f := sprig.TxtFuncMap()
	delete(f, "env")
	delete(f, "expandenv")

	// Add some extra functionality
	extra := template.FuncMap{
		"toToml":   chartutil.ToToml,
		"toYaml":   chartutil.ToYaml,
		"fromYaml": chartutil.FromYaml,
		"toJson":   chartutil.ToJson,
		"fromJson": chartutil.FromJson,
		"ToYAML":   strvals.ToYAML,
	}

	for k, v := range extra {
		f[k] = v
	}

	return f
}

const Version = "0.0.1"

// Usage is a docopt-formatted specification for this application's command line interface.
const Usage = `Usage:
  gotpl [--values-from-stdin] [--set <key=value>...] [-f <file>...] [--name <name>] [--namespace <namespace>] [--template <template>...] <chart>
  gotpl -h | --help
  gotpl -v | --version
Options:
  -f <file> --values <file> specify values in a YAML file or a URL(can specify multiple) (default [])
  --set <key=value>         set values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)
  --values-from-stdin       set values in a YAML file which is read from stdin (default false)
  -n <name> --name <name>   release name. If unspecified, it will autogenerate one for you
  --namespace <namespace>   namespace to put the release into (default "default")
  --template <template>     select template(s) to generate, and the order in which they are generated (default all)
  -h --help                 Show usage information
  -v --version              Show version information`

// Reads a YAML document from the values_in stream, uses it as values
// for the tpl_files templates and writes the executed templates to
// the out stream.
func ExecuteTemplates(variables map[string]interface{}, out io.Writer, tpl_files ...string) error {
	var notFirstFile bool 
	notFirstFile = false
	for _, tpl_file := range tpl_files {
		tpl, err := template.New(path.Base(tpl_file)).Funcs(FuncMap()).Option("missingkey=error").ParseFiles(tpl_file)
		if err != nil {
			return fmt.Errorf("Failed to parse template(s): %v", err)
		}

		var testbuilder strings.Builder
		err = tpl.Execute(&testbuilder, variables)
		if err != nil {
			return fmt.Errorf("Failed to execute template(s): %v", err)
		}

		if notFirstFile {
			out.Write([]byte("\n"))
			out.Write([]byte("---\n"))
		}

		tpl.Execute(out, variables)

		notFirstFile = true
	}
	return nil
}

func getEnvironment(data []string) map[string]string {
	items := make(map[string]string)
	for _, item := range data {
		key, val := getKeyVal(item)
		items[key] = val
	}
	return items
}

func getKeyVal(item string) (key, val string) {
	splits := strings.Split(item, "=")
	key = splits[0]
	val = splits[1]
	return
}

func main() {
	arguments, _ := docopt.Parse(Usage, nil, true, Version, false)

	var err error

	chart := arguments["<chart>"].(string)
	chart_file := chart + "/Chart.yaml"
	chart_templates_dir := chart + "/templates"
	chart_templates_glob := chart_templates_dir + "/*.yaml"
	var chart_templates []string
	chart_templates, err = filepath.Glob(chart_templates_glob)
	var selected_templates []string = []string{}
	if arguments["--template"] != nil {
		for _, selected_template_name := range arguments["--template"].([]string) {
			var i int = 0
			for _, available_template := range chart_templates {
				var selected_template string = chart_templates_dir + "/" + selected_template_name + ".yaml"
				if selected_template == available_template {
					selected_templates = append(selected_templates, selected_template)
					i = 1
				}
			}
			if i == 0 {
				log.Println(fmt.Errorf("no such template: ", selected_template_name))
				os.Exit(1)
			}
		}
	}

	if len(selected_templates) == 0 {
		selected_templates = chart_templates
	}

	var chartvalues map[string]interface{}
	if _, err := os.Stat(chart_file); err == nil {
		var chart_file_in io.Reader
		chart_file_in, err = os.Open(chart_file)
		if err != nil {
			log.Println(fmt.Errorf("Failed to open file descriptor to chart file %v: %v", chart_file, err))
			os.Exit(1)
		}
		chartbuf := bytes.NewBuffer(nil)
		_, err = io.Copy(chartbuf, chart_file_in)
		if err != nil {
			log.Println(fmt.Errorf("Failed to copy yaml into buffer for chart file %v: %v", chart_file, err))
			os.Exit(1)
		}

		err = yaml.Unmarshal(chartbuf.Bytes(), &chartvalues)
		if err != nil {
			log.Println(fmt.Errorf("Failed to parse yaml from chart file %v: %v", chart_file, err))
			os.Exit(1)
		}
	}

	buf := bytes.NewBuffer(nil)

	var values map[string]interface{}
	if arguments["--values-from-stdin"].(bool) {
		var values_in io.Reader = os.Stdin
		_, err = io.Copy(buf, values_in)
		if err != nil {
			log.Println(fmt.Errorf("Failed to read standard input: %v", err))
			os.Exit(1)
		}

		err = yaml.Unmarshal(buf.Bytes(), &values)
		if err != nil {
			log.Println(fmt.Errorf("Failed to parse yaml from standard input: %v", err))
			os.Exit(1)
		}
	}

	if arguments["--values"] != nil {
		var values_files_in []string = arguments["--values"].([]string)		
		for _, value_file := range values_files_in {
			var value_file_in io.Reader
			value_file_in, err = os.Open(value_file)
			if err != nil {
				log.Println(fmt.Errorf("Failed to open file descriptor to file %v: %v", value_file, err))
				os.Exit(1)
			}
			setbuf := bytes.NewBuffer(nil)
			_, err = io.Copy(setbuf, value_file_in)
			if err != nil {
				log.Println(fmt.Errorf("Failed to copy yaml into buffer for file %v: %v", value_file, err))
				os.Exit(1)
			}
		
			var setvalues map[string]interface{}
			err = yaml.Unmarshal(setbuf.Bytes(), &setvalues)
			if err != nil {
				log.Println(fmt.Errorf("Failed to parse yaml from file %v: %v", value_file, err))
				os.Exit(1)
			}

			if err := mergo.Merge(&values, setvalues, mergo.WithOverride); err != nil {
				log.Println(fmt.Errorf("Failed to merge yaml from file %v into general values map: %v", value_file, err))
				os.Exit(1)
			}
		}
	}

	if arguments["--set"] != nil {
		var flag_values_in []string = arguments["--set"].([]string)		
		for _, flag_value := range flag_values_in {
			yamlstr, err := strvals.ToYAML(flag_value)
			if err != nil {
				log.Println(fmt.Errorf("Failed to convert set argument %v into yaml: %v", flag_value, err))
				os.Exit(1)
			}
			setbuf := bytes.NewBuffer(nil)
			yamlbytes := []byte(yamlstr)
			pr, pw := io.Pipe()

			go func() {
				// close the writer, so the reader knows there's no more data
				defer pw.Close()
			
				pw.Write(yamlbytes)
			}()
			_, err = io.Copy(setbuf, pr)
			if err != nil {
				log.Println(fmt.Errorf("Failed to copy yaml into buffer for set argument %v: %v", flag_value, err))
				os.Exit(1)
			}
		
			var setvalues map[string]interface{}
			err = yaml.Unmarshal(setbuf.Bytes(), &setvalues)
			if err != nil {
				log.Println(fmt.Errorf("Failed to parse yaml for set argument %v: %v", flag_value, err))
				os.Exit(1)
			}

			if err := mergo.Merge(&values, setvalues, mergo.WithOverride); err != nil {
				log.Println(fmt.Errorf("Failed to merge yaml from set argument %v into general values map: %v", flag_value, err))
				os.Exit(1)
			}
		}
	}

	var releasevars map[string]string = make(map[string]string)
	if arguments["--name"] != nil {
		releasevars["Name"] = arguments["--name"].(string)
	}

	if arguments["--namespace"] != nil {
		releasevars["Namespace"] = arguments["--namespace"].(string)
	}

	var variables map[string]interface{} = make(map[string]interface{})
	variables["Values"] = values
	variables["Env"] = getEnvironment(os.Environ())
	variables["Release"] = releasevars
	variables["Chart"] = chartvalues

	err = ExecuteTemplates(variables, os.Stdout, selected_templates...)
	if err != nil {
		log.Println(fmt.Errorf("Failed to run ExecuteTemplates: %v", err))
		os.Exit(1)
	}
}
