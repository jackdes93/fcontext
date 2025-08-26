package fcontext

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/joho/godotenv"
)

const (
	DevEnv = "dev"
	StgEnv = "stg"
	PrdEnv = "prd"
)

type ServiceContext interface {
	Load() error
	MustGet(id string) any
	Get(id string) (any, bool)
	Logger(prefix string) Logger
	EnvName() string
	GetName() string
	Stop() error
	OutEnv()
}

type serviceCtx struct {
	name       string
	env        string
	envFile    string
	components []Component
	store      map[string]Component
	cmdLine    *AppFlatSet
	logger     Logger
}

func New(opts ...Option) ServiceContext {
	sv := &serviceCtx{
		store: make(map[string]Component),
	}

	for _, opt := range opts {
		opt(sv)
	}
	sv.initFlags()
	sv.cmdLine = newFlagSet(sv.name, flag.CommandLine)
	sv.parseFlags()

	if sv.logger == nil {
		sv.logger = newZeroLogger(sv.name)
	}
	return sv
}

func (s *serviceCtx) initFlags() {
	flag.StringVar(&s.env, "app-env", DevEnv, "Env for service. Ex: dev | stg | prd")
	flag.StringVar(&s.envFile, "env-file", "", "Path to .env file")
	for _, c := range s.components {
		c.InitFlags()
	}
}

func (s *serviceCtx) parseFlags() {
	s.cmdLine.Parse([]string{})
	envFile := s.envFile
	if envFile == "" {
		if v := os.Getenv("ENV_FILE"); v != "" {
			envFile = v
		} else {
			envFile = ".env"
		}
	}

	if st, err := os.Stat(envFile); err == nil && !st.IsDir() {
		if err := godotenv.Load(envFile); err != nil {
			log.Fatalf("Loading env(%s): %s", envFile, err.Error())
		}
	} else if envFile != ".env" {
		log.Fatalf("Loading env(%s): %v", envFile, err)
	}
}

func (s *serviceCtx) Get(id string) (any, bool) {
	c, ok := s.store[id]
	if !ok {
		return nil, false
	}
	return c, true
}

func (s *serviceCtx) MustGet(id string) any {
	v, ok := s.Get(id)
	if !ok {
		panic(fmt.Sprintf("cannot get %s", id))
	}
	return v
}

func (s *serviceCtx) Logger(prefix string) Logger {
	return s.logger.WithPrefix(prefix)
}

func (s *serviceCtx) Load() error {
	s.logger.Info("Service context is loading...")

	sort.SliceStable(s.components, func(i, j int) bool {
		return componentOrder(s.components[i]) < componentOrder(s.components[j])
	})
	ctx := context.Background()
	activated := make([]Component, 0, len(s.components))

	for _, c := range s.components {
		if err := c.Activate(ctx, s); err != nil {
			s.logger.Error("Activate failed for %s: %v; rolling back", c.ID(), err)
			for k := len(activated) - 1; k >= 0; k-- {
				_ = activated[k].Stop(ctx)
			}
			return err
		}
		activated = append(activated, c)
	}
	s.logger.Info("Service context loaded")
	return nil
}

func (s *serviceCtx) Stop() error {
	s.logger.Info("Stopping service context")
	ctx := context.Background()

	var errs []error
	for i := len(s.components) - 1; i >= 0; i-- {
		if err := s.components[i].Stop(ctx); err != nil {
			s.logger.Error("Stop %s error: %v", s.components[i].ID(), err)
			errs = append(errs, err)
		}
	}
	s.logger.Info("Service context stopped")
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (s *serviceCtx) GetName() string { return s.name }
func (s *serviceCtx) EnvName() string { return s.env }
func (s *serviceCtx) OutEnv()         { s.cmdLine.GetSampleEnvs() }

func GetAs[T any](sv ServiceContext, id string) (T, bool) {
	var zero T
	v, ok := sv.Get(id)
	if !ok {
		return zero, false
	}
	x, ok := v.(T)
	return x, ok
}
