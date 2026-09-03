#!/bin/sh
set -eu

required_files='LICENSE.de.md LICENSE.md LICENSE_HISTORY.md NOTICE.md COPYRIGHT.md TRADEMARKS.md THIRD_PARTY_NOTICES.md'

for file in $required_files; do
  if [ ! -s "$file" ]; then
    echo "Required legal file is missing or empty: $file" >&2
    exit 1
  fi
done

grep -Fq 'Quantum Runtime Community Source Lizenz 1.0' LICENSE.de.md
grep -Fq 'Quantum Runtime Community Source License 1.0' LICENSE.md
grep -Fq 'Source-Available-Lizenz' LICENSE.de.md
grep -Fq 'not an Open Source Initiative approved open-source license' LICENSE.md
grep -Fq 'THIRD_PARTY_NOTICES.md' LICENSE.de.md
grep -Fq 'THIRD_PARTY_NOTICES.md' LICENSE.md

echo 'Quantum Runtime legal-file verification passed.'
