from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    target = Path(path)
    text = target.read_text(encoding="utf-8")
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one match for {old!r}, got {count}")
    target.write_text(text.replace(old, new, 1), encoding="utf-8")


replace_once(
    "internal/modelregistry/registry_test.go",
    "if registry.Len() != 3 {\n\t\tt.Fatalf(\"expected 3 builtin manifests, got %d\", registry.Len())\n\t}",
    "if registry.Len() != 4 {\n\t\tt.Fatalf(\"expected 4 builtin manifests, got %d\", registry.Len())\n\t}",
)
replace_once(
    "internal/modelregistry/registry_test.go",
    "if len(entries) != 3 {",
    "if len(entries) != 4 {",
)
replace_once(
    "internal/httpapi/registry_routes_test.go",
    "if payload.Count != 3 || len(payload.Models) != 3 {",
    "if payload.Count != 4 || len(payload.Models) != 4 {",
)

print("Adjusted builtin registry cardinality tests for the new 0.3 reference manifest")
