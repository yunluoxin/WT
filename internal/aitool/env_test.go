package aitool

import "os"

func lookupEnv(key string) (string, bool) { return os.LookupEnv(key) }
func setEnv(key, v string)                { _ = os.Setenv(key, v) }
func unsetEnvImpl(key string)             { _ = os.Unsetenv(key) }
