# Quantum Runtime Linux installation

Status: `0.2.0-alpha.2`

This installation profile is intentionally non-destructive. Quantum Runtime is installed next to an existing local Ollama service and becomes an optional application-facing runtime layer. Ollama remains externally owned and is never pulled, upgraded, restarted, renamed or removed by the Runtime installer.

## Package layout

```text
/usr/local/bin/quantum-runtime
/usr/local/bin/quantum-runtime-installer
/etc/quantum-runtime/quantum-runtime.env
/etc/quantum-runtime/.managed.json
/etc/systemd/system/quantum-runtime.service
```

The service runs as the dedicated `quantum-runtime` system user and group. The default listener remains `127.0.0.1:11450` and the adoption backend remains `http://127.0.0.1:11434`.

## Preflight

Preflight is inspect-only:

```bash
sudo ./quantum-runtime-installer preflight
sudo ./quantum-runtime-installer preflight --json
```

It checks systemd availability, the local Ollama version endpoint and existing Quantum Runtime ownership. It does not create files, users, groups, services or models.

Ownership states are:

- `managed`: this installer owns the Runtime binary and unit through `.managed.json`
- `external`: Runtime files exist but were not created by this installer; install/repair/uninstall fail closed
- `disabled`: no Runtime installation is detected

Ollama is reported as `external` when reachable and `disabled` when it is not reachable. Quantum Runtime never reports Ollama as `managed`.

## Install

From an extracted release archive:

```bash
sudo ./quantum-runtime-installer install \
  --runtime-binary ./quantum-runtime
```

Installation performs these steps:

1. preflight with no mutation
2. require an already reachable local Ollama adoption backend
3. refuse to overwrite externally owned Runtime files
4. create the dedicated Runtime system identity when missing
5. preserve an existing `/etc/quantum-runtime/quantum-runtime.env`
6. atomically install the Runtime binary and hardened systemd unit
7. write a root-owned ownership marker
8. reload systemd and enable/start only `quantum-runtime.service`
9. require both `/healthz` and `/readyz`
10. restore the previous Runtime binary, unit, config and ownership marker when activation fails

No model mutation endpoint is enabled by the installer.

`--no-start` installs files without enabling or starting the service. It is intended for controlled image construction and does not perform the activation health gate.

## Status and repair

```bash
sudo ./quantum-runtime-installer status
sudo ./quantum-runtime-installer status --json
sudo ./quantum-runtime-installer repair --runtime-binary ./quantum-runtime
```

Repair is available only for installer-managed Runtime instances and uses the same transactional install path.

## Uninstall

```bash
sudo ./quantum-runtime-installer uninstall
```

The normal uninstall removes only installer-managed Runtime binary/unit/marker files. It preserves the Runtime environment file, Ollama, all Ollama models and all application data.

If the environment file was originally created by this installer, it may be explicitly removed with:

```bash
sudo ./quantum-runtime-installer uninstall --purge-managed-config
```

A pre-existing configuration file is never removed by this flag.

## Ember CoreUI adoption

Ember CoreUI remains an independent Repack. Quantum Runtime is optional.

The current CoreUI server path uses `STU_EMBER_OLLAMA_URL`; the adoption profile also exports `COREUI_OLLAMA_URL` as the stable installer/profile integration name. Both point to the same endpoint.

Route CoreUI through Quantum Runtime:

```text
COREUI_OLLAMA_URL=http://127.0.0.1:11450/api/chat
STU_EMBER_OLLAMA_URL=http://127.0.0.1:11450/api/chat
```

Return to direct Ollama:

```text
COREUI_OLLAMA_URL=http://127.0.0.1:11434/api/chat
STU_EMBER_OLLAMA_URL=http://127.0.0.1:11434/api/chat
```

The repository provides both profiles under `profiles/coreui/`. The installer can print either one without modifying CoreUI:

```bash
./quantum-runtime-installer coreui-profile --mode runtime
./quantum-runtime-installer coreui-profile --mode ollama
```

Applying a profile to a CoreUI installation remains an explicit operator action. Quantum Runtime does not edit CoreUI databases, accounts, sessions, uploads, secrets or model data.

## Remote exposure

The generated Runtime configuration is loopback-only. Existing Runtime validation already refuses a non-loopback listen address unless an authentication token is configured. The installer does not create or expose a remote listener automatically.
