// Command jamii is the build and deploy helper for the Jamii Savings
// integrations. scripts/build-tools.sh builds it into .tools/bin/jamii.
//
//	jamii package-car --mi-root mi --version 1.0.0 --out mi/target/cars [--validate-only]
//	jamii apim-consumer [--apim URL] [--gateway URL] [--env-file FILE]
//	jamii mi-apps state|undeployed|active [Name:version ...]   (reads GET /applications JSON on stdin)
//	jamii json <path>                                          (reads JSON on stdin)
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"jamiisavings/tools/apimconsumer"
	"jamiisavings/tools/carpkg"
	"jamiisavings/tools/jsonq"
	"jamiisavings/tools/miapps"
)

const usage = `usage: jamii <command> [args]

commands:
  package-car     validate MI artifacts and package them into CARs
  apim-consumer   onboard the demo app via the Developer Portal REST API
  mi-apps         interpret MI's GET /applications response (stdin)
  json            print a value from JSON on stdin, e.g. jamii json error.code
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "package-car":
		err = packageCar(args)
	case "apim-consumer":
		err = apimConsumer(args)
	case "mi-apps":
		err = miApps(args)
	case "json":
		err = jsonCmd(args)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "jamii "+cmd+": "+err.Error())
		os.Exit(1)
	}
}

func packageCar(args []string) error {
	fs := flag.NewFlagSet("package-car", flag.ExitOnError)
	miRoot := fs.String("mi-root", "mi", "the mi/ directory")
	version := fs.String("version", "", "CAR version (required)")
	out := fs.String("out", "mi/target/cars", "output directory")
	validateOnly := fs.Bool("validate-only", false, "validate without packaging")
	_ = fs.Parse(args)
	if *version == "" {
		return fmt.Errorf("--version is required")
	}
	return carpkg.Run(*miRoot, *version, *out, *validateOnly, os.Stderr)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func apimConsumer(args []string) error {
	fs := flag.NewFlagSet("apim-consumer", flag.ExitOnError)
	apim := fs.String("apim", envOr("APIM_URL", "https://localhost:9443"), "APIM base URL")
	gateway := fs.String("gateway", envOr("APIM_GATEWAY_URL", "https://localhost:8243"), "gateway base URL")
	envFile := fs.String("env-file", "tests/postman/apim.env.json", "Postman environment to write")
	_ = fs.Parse(args)
	res, err := apimconsumer.Run(apimconsumer.Config{
		APIM: *apim, Gateway: *gateway, EnvFile: *envFile, Log: os.Stderr,
		User:     envOr("APIM_ADMIN_USER", "admin"),
		Password: envOr("APIM_ADMIN_PASSWORD", "admin"),
	})
	if err != nil {
		return err
	}
	fmt.Print(apimconsumer.Commands(*gateway, res))
	return nil
}

func miApps(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: jamii mi-apps state|undeployed|active [Name:version ...]")
	}
	switch args[0] {
	case "state":
		s, err := miapps.State(os.Stdin, args[1:])
		if err == nil {
			fmt.Println(s)
		}
		return err
	case "undeployed":
		done, err := miapps.Undeployed(os.Stdin, args[1:])
		if err == nil && !done {
			os.Exit(1) // still deployed: a normal "not yet" answer, not an error
		}
		return err
	case "active":
		s, err := miapps.Active(os.Stdin)
		if err == nil {
			fmt.Println(s)
		}
		return err
	}
	return fmt.Errorf("unknown mi-apps mode %q", args[0])
}

func jsonCmd(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: jamii json <path>")
	}
	values, err := jsonq.Query(os.Stdin, args[0])
	if err != nil {
		return err
	}
	fmt.Println(strings.Join(values, "\n"))
	return nil
}
