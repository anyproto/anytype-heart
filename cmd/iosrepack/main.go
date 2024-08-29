package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	repack            = "repack"
	iosArm64          = "ios_arm64.a"
	iosArm64Sim       = "ios_arm64_sim.a"
	iosX86Sim         = "ios_x86_sim.a"
	iosArm64Repack    = "iosArm64"
	iosArm64SimRepack = "iosArm64Sim"
	iosX86SimRepack   = "iosX86Sim"
	combined          = "libcombined.a"
)

type iosContext struct {
	tantivyArm64    string
	tantivyArm64Sim string
	tantivyX86Sim   string
	iosLib          string
}

func main() {
	err := delegateMain()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(-1)
	}
}

func delegateMain() error {
	ctx := new(iosContext)
	ctx.tantivyArm64 = "deps/libs/ios-arm64/libtantivy_go.a"
	ctx.tantivyArm64Sim = "deps/libs/ios-arm64-sim/libtantivy_go.a"
	ctx.tantivyX86Sim = "deps/libs/ios-amd64/libtantivy_go.a"
	ctx.iosLib = "dist/ios/Lib.xcframework"

	iosArm64Lib, iosSimLib, err := thinIosLibs(ctx)
	if err != nil {
		return err
	}

	err = repackIosLibsWithTantivy(ctx)
	if err != nil {
		return err
	}

	err = removeOldIosLibs(iosArm64Lib, iosSimLib)
	if err != nil {
		return err
	}

	err = createNewIosLibs(iosArm64Lib, iosSimLib)
	if err != nil {
		return err
	}

	return nil
}

func createNewIosLibs(iosArm64Lib string, iosSimLib string) error {
	defer func() {
		_ = execute("Cleanup:", "rm", "-rf", repack)
		_ = execute("Cleanup:", "rm", iosArm64)
		_ = execute("Cleanup:", "rm", iosArm64Sim)
		_ = execute("Cleanup:", "rm", iosX86Sim)
	}()

	err := execute("Error creating lib:", "lipo", "-create",
		filepath.Join(repack, iosArm64Repack, combined), "-output", "Lib")
	if err != nil {
		return err
	}

	err = execute("Move created lib:", "mv", "Lib", iosArm64Lib)
	if err != nil {
		return err
	}

	err = execute("Error creating lib:", "lipo", "-create",
		filepath.Join(repack, iosArm64SimRepack, combined),
		filepath.Join(repack, iosX86SimRepack, combined), "-output", "Lib")
	if err != nil {
		return err
	}

	err = execute("Move created lib:", "mv", "Lib", iosSimLib)
	if err != nil {
		return err
	}

	return nil
}

func removeOldIosLibs(iosArm64Lib string, iosSimLib string) error {
	err := execute("Error removing lib:", "rm", iosArm64Lib)
	if err != nil {
		return err
	}

	err = execute("Error removing lib:", "rm", iosSimLib)
	if err != nil {
		return err
	}
	return nil
}

func thinIosLibs(ctx *iosContext) (string, string, error) {
	iosArm64Lib := filepath.Join(ctx.iosLib, "ios-arm64", "Lib.framework", "Lib")
	err := execute("Error extracting lib:", "lipo", iosArm64Lib, "-thin", "arm64", "-output", iosArm64)
	if err != nil {
		return "", "", err
	}

	iosSimLib := filepath.Join(ctx.iosLib, "ios-arm64_x86_64-simulator", "Lib.framework", "Lib")
	err = execute("Error extracting lib:", "lipo", iosSimLib, "-thin", "arm64", "-output", iosArm64Sim)
	if err != nil {
		return "", "", err
	}

	err = execute("Error extracting lib:", "lipo", iosSimLib, "-thin", "x86_64", "-output", iosX86Sim)
	if err != nil {
		return "", "", err
	}
	return iosArm64Lib, iosSimLib, nil
}

func repackIosLibsWithTantivy(ctx *iosContext) error {
	err := os.MkdirAll(repack, os.ModePerm)
	if err != nil {
		return err
	}

	err = os.Chdir(repack)
	if err != nil {
		return err
	}

	err = repackLib(iosArm64Repack, ctx.tantivyArm64, iosArm64)
	if err != nil {
		return err
	}

	err = repackLib(iosArm64SimRepack, ctx.tantivyArm64Sim, iosArm64Sim)
	if err != nil {
		return err
	}

	err = repackLib(iosX86SimRepack, ctx.tantivyX86Sim, iosX86Sim)
	if err != nil {
		return err
	}

	err = os.Chdir("..")
	if err != nil {
		return err
	}
	return nil
}

func repackLib(dir string, tantivyLib string, iosLib string) error {
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return err
	}

	err = os.Chdir(dir)
	if err != nil {
		return err
	}

	err = execute("Error extracting lib:", "ar", "-x", filepath.Join("..", "..", tantivyLib))
	if err != nil {
		return err
	}

	err = execute("Error extracting lib:", "ar", "-x", filepath.Join("..", "..", iosLib))
	if err != nil {
		return err
	}

	oFiles, err := filepath.Glob("*.o")
	if err != nil {
		return err
	}

	err = execute("Error combine lib:", "ar", append([]string{"-qc", combined}, oFiles...)...)
	if err != nil {
		return err
	}

	err = os.Chdir("..")
	if err != nil {
		return err
	}
	return nil
}

func execute(errText string, name string, arg ...string) error {
	cmd := exec.Command(name, arg...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		fmt.Println(errText, err)
		return err
	}
	return nil
}
