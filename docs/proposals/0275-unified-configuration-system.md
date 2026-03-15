# Unified Configuration System

**Authors**: @Iduranga-Uwanpriya  
**Reviewers**: @Mirage20  
**Created Date**: 2025-09-23  
**Status**: Draft  
**Related Issues/PRs**: #275

---

## Summary
Introduce a single, consistent configuration system across all OpenChoreo components using the [koanf](https://github.com/knadh/koanf) library.  
This will unify the way defaults, config files, environment variables, and CLI flags are handled, making the system easier to use, maintain, and deploy.

---

## Motivation
Currently, each component relies on Go’s `flag` package with no shared structure.  
This creates:
- Inconsistent behavior across components  
- Repeated parsing and validation code  
- Difficulty for operators who need to run the system in different environments  

A unified approach will make configuration predictable, 12-Factor friendly, and easier for both developers and users.

---

## Proposal
- Adopt **koanf** as the configuration engine.  
- Configuration precedence (lowest → highest):  
  1. Built-in defaults  
  2. Config file (`config.yaml` by default, override with `--config`)  
  3. Environment variables (prefix `OPENCHOREO_`, nested keys mapped with `__`)  
  4. CLI flags (minimal curated overrides + optional `--set key=value`)  
- Define a single `Config` struct with sub-sections (`Server`, `Logging`, `Telemetry`, `Controller`, `Observer`, `API`).  
- Provide sample files and clear documentation for local, Docker, and Kubernetes setups.  
- Keep old flags temporarily with deprecation warnings to ease migration.

---

## Alternatives
- **Viper**: rich feature set but heavy dependencies and larger binaries.  
- **envconfig**: simple mapping from env vars, but lacks file/flag support.  
- **Custom solution**: would require more maintenance effort with little benefit.  

---

## Impact
**Pros**  
- Consistent behavior across all binaries  
- Lightweight dependency (koanf)  
- Works well in containers and Kubernetes  
- Easier to document and support  

**Cons**  
- Introduces a new dependency  
- Requires migration from legacy flags  

**Migration Plan**  
- Keep old flags for one minor release, showing a clear deprecation warning at startup.  
- Document mapping from old → new keys.  
- Remove deprecated flags in the following minor release.  

---

## Implementation Plan
1. Add a new `/pkg/config` package:  
   - `schema.go` (Config struct)  
   - `defaults.go` (sane defaults)  
   - `loader.go` (koanf merge logic)  
   - `validate.go` (basic input checks)  
   - Unit tests for precedence and validation  
2. Wire the **API server** to use the new config system first.  
3. Provide `docs/configuration.md` and `samples/config.yaml`.  
4. Roll out to **observer** and **controller**.  
5. Remove deprecated flags in the next minor release.  

---

## References
- [koanf](https://github.com/knadh/koanf)  
- [12-Factor App – Config](https://12factor.net/config)  
- [Issue #275](https://github.com/openchoreo/openchoreo/issues/275)
