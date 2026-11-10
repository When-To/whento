# Third-Party Licenses

This document lists all third-party dependencies used in WhenTo and their respective licenses.

WhenTo is licensed under the **Business Source License 1.1** (BSL). All dependencies listed below use permissive licenses that are fully compatible with BSL.

---

## License Compatibility Summary

✅ **All dependencies use BSL-compatible licenses:**

- **MIT License** - Majority of dependencies
- **Apache License 2.0** - Prometheus client, OTP, iCal, Swagger tooling, TypeScript, Playwright
- **BSD-2-Clause / BSD-3-Clause** - Redis client, WebAuthn, protobuf, Google and `golang.org/x` packages
- **ISC License** - Similar to MIT, fully permissive
- **SIL Open Font License 1.1** - The bundled Inter font

❌ **No copyleft licenses** (GPL, AGPL, LGPL) are used in this project.

---

## Go Dependencies (Backend)

The backend is a Go workspace of two modules — `github.com/whento/whento` (root) and
`github.com/whento/pkg` (`pkg/`, wired in via `replace`). Both ship inside the same
single binary, so the tables below are the union of the two `go.mod` files.
`github.com/whento/pkg` itself is first-party and is not listed.

### Direct Dependencies

| Package | Version | License |
|---------|---------|---------|
| [github.com/arran4/golang-ical](https://github.com/arran4/golang-ical) | v0.3.5 | Apache-2.0 |
| [github.com/go-chi/chi/v5](https://github.com/go-chi/chi) | v5.5.0 | MIT |
| [github.com/go-playground/tz](https://github.com/go-playground/tz) | v0.0.1 | MIT |
| [github.com/go-playground/validator/v10](https://github.com/go-playground/validator) | v10.30.6 | MIT |
| [github.com/go-webauthn/webauthn](https://github.com/go-webauthn/webauthn) | v0.17.4 | BSD-3-Clause |
| [github.com/golang-jwt/jwt/v5](https://github.com/golang-jwt/jwt) | v5.3.1 | MIT |
| [github.com/google/uuid](https://github.com/google/uuid) | v1.6.0 | BSD-3-Clause |
| [github.com/jackc/pgx/v5](https://github.com/jackc/pgx) | v5.11.0 | MIT |
| [github.com/joho/godotenv](https://github.com/joho/godotenv) | v1.5.1 | MIT |
| [github.com/pquerna/otp](https://github.com/pquerna/otp) | v1.5.0 | Apache-2.0 |
| [github.com/prometheus/client_golang](https://github.com/prometheus/client_golang) | v1.25.0 | Apache-2.0 |
| [github.com/redis/go-redis/v9](https://github.com/redis/go-redis) | v9.23.0 | BSD-2-Clause |
| [github.com/rickar/cal/v2](https://github.com/rickar/cal/tree/v2.1.32) | v2.1.32 | BSD-3-Clause |
| [github.com/skip2/go-qrcode](https://github.com/skip2/go-qrcode) | v0.0.0-20200617195104 | MIT |
| [github.com/swaggo/http-swagger/v2](https://github.com/swaggo/http-swagger) | v2.0.2 | MIT |
| [github.com/swaggo/swag](https://github.com/swaggo/swag) | v1.16.6 | MIT |
| [golang.org/x/crypto](https://golang.org/x/crypto) | v0.58.0 | BSD-3-Clause |
| [golang.org/x/sync](https://golang.org/x/sync) | v0.24.0 | BSD-3-Clause |
| [golang.org/x/time](https://golang.org/x/time) | v0.16.0 | BSD-3-Clause |

### Indirect Dependencies

| Package | Version | License |
|---------|---------|---------|
| github.com/KyleBanks/depth | v1.2.1 | MIT |
| github.com/beorn7/perks | v1.0.1 | MIT |
| github.com/boombuler/barcode | v1.1.0 | MIT |
| github.com/cespare/xxhash/v2 | v2.3.0 | MIT |
| github.com/fxamacker/cbor/v2 | v2.9.2 | MIT |
| github.com/gabriel-vasile/mimetype | v1.4.16 | MIT |
| github.com/go-openapi/jsonpointer | v1.0.0 | Apache-2.0 |
| github.com/go-openapi/jsonreference | v1.0.0 | Apache-2.0 |
| github.com/go-openapi/spec | v0.22.9 | Apache-2.0 |
| github.com/go-openapi/swag/conv | v0.28.0 | Apache-2.0 |
| github.com/go-openapi/swag/jsonutils | v0.28.0 | Apache-2.0 |
| github.com/go-openapi/swag/loading | v0.28.0 | Apache-2.0 |
| github.com/go-openapi/swag/pools | v0.28.0 | Apache-2.0 |
| github.com/go-openapi/swag/stringutils | v0.28.0 | Apache-2.0 |
| github.com/go-openapi/swag/typeutils | v0.28.0 | Apache-2.0 |
| github.com/go-openapi/swag/yamlutils | v0.28.0 | Apache-2.0 |
| github.com/go-playground/locales | v0.14.2 | MIT |
| github.com/go-playground/universal-translator | v0.18.2 | MIT |
| github.com/go-viper/mapstructure/v2 | v2.5.0 | MIT |
| github.com/go-webauthn/x | v0.2.8 | BSD-3-Clause |
| github.com/google/go-tpm | v0.9.8 | Apache-2.0 |
| github.com/jackc/pgpassfile | v1.0.0 | MIT |
| github.com/jackc/pgservicefile | v0.0.0-20240606120523 | MIT |
| github.com/jackc/puddle/v2 | v2.2.3 | MIT |
| github.com/kylelemons/godebug | v1.1.0 | Apache-2.0 |
| github.com/leodido/go-urn | v1.5.0 | MIT |
| github.com/munnerz/goautoneg | v0.0.0-20191010083416 | BSD-3-Clause |
| github.com/pandatix/go-abnf | v0.5.3 | MIT |
| github.com/philhofer/fwd | v1.2.0 | MIT |
| github.com/prometheus/client_model | v0.6.3 | Apache-2.0 |
| github.com/prometheus/common | v0.73.1 | Apache-2.0 |
| github.com/prometheus/procfs | v0.23.0 | Apache-2.0 |
| github.com/swaggo/files/v2 | v2.0.2 | MIT |
| github.com/tinylib/msgp | v1.6.4 | MIT |
| github.com/x448/float16 | v0.8.4 | MIT |
| go.uber.org/atomic | v1.12.0 | MIT |
| go.yaml.in/yaml/v3 | v3.0.5 | MIT AND Apache-2.0* |
| golang.org/x/mod | v0.40.0 | BSD-3-Clause |
| golang.org/x/sys | v0.49.0 | BSD-3-Clause |
| golang.org/x/text | v0.43.0 | BSD-3-Clause |
| golang.org/x/tools | v0.49.0 | BSD-3-Clause |
| google.golang.org/protobuf | v1.36.12 | BSD-3-Clause |

\* *go.yaml.in/yaml/v3 ships a single LICENSE covering the package under both MIT and Apache-2.0; both are permissive and BSL-compatible.*

---

## NPM Dependencies (Frontend)

### Production Dependencies

| Package | Version | License |
|---------|---------|---------|
| [@fontsource-variable/inter](https://www.npmjs.com/package/@fontsource-variable/inter) | 5.3.0 | OFL-1.1† |
| [@vueuse/core](https://www.npmjs.com/package/@vueuse/core) | 15.0.0 | MIT |
| [axios](https://www.npmjs.com/package/axios) | 1.21.0 | MIT |
| [countries-and-timezones](https://www.npmjs.com/package/countries-and-timezones) | 3.11.0 | MIT |
| [date-holidays](https://www.npmjs.com/package/date-holidays) | 3.38.0 | ISC* |
| [pinia](https://www.npmjs.com/package/pinia) | 4.1.0 | MIT |
| [vue](https://www.npmjs.com/package/vue) | 3.5.44 | MIT |
| [vue-i18n](https://www.npmjs.com/package/vue-i18n) | 11.4.16 | MIT |
| [vue-router](https://www.npmjs.com/package/vue-router) | 5.4.0 | MIT |

\* *date-holidays: Code is ISC licensed, holiday data is CC-BY-3.0 (Creative Commons Attribution)*

† *@fontsource-variable/inter: the Inter font files are licensed under the SIL Open Font License 1.1.*

### Development Dependencies

| Package | Version | License |
|---------|---------|---------|
| [@eslint/js](https://www.npmjs.com/package/@eslint/js) | 10.0.1 | MIT |
| [@playwright/test](https://www.npmjs.com/package/@playwright/test) | 1.64.1 | Apache-2.0 |
| [@tailwindcss/postcss](https://www.npmjs.com/package/@tailwindcss/postcss) | 4.3.3 | MIT |
| [@types/node](https://www.npmjs.com/package/@types/node) | 24.19.2 | MIT |
| [@types/pg](https://www.npmjs.com/package/@types/pg) | 8.23.1 | MIT |
| [@vitejs/plugin-vue](https://www.npmjs.com/package/@vitejs/plugin-vue) | 6.0.10 | MIT |
| [@vitest/coverage-v8](https://www.npmjs.com/package/@vitest/coverage-v8) | 5.0.5 | MIT |
| [@vue/test-utils](https://www.npmjs.com/package/@vue/test-utils) | 2.5.1 | MIT |
| [eslint](https://www.npmjs.com/package/eslint) | 10.14.0 | MIT |
| [eslint-config-prettier](https://www.npmjs.com/package/eslint-config-prettier) | 10.1.8 | MIT |
| [eslint-plugin-vue](https://www.npmjs.com/package/eslint-plugin-vue) | 10.12.0 | MIT |
| [eslint-plugin-vuejs-accessibility](https://www.npmjs.com/package/eslint-plugin-vuejs-accessibility) | 2.6.3 | MIT |
| [globals](https://www.npmjs.com/package/globals) | 17.13.0 | MIT |
| [jsdom](https://www.npmjs.com/package/jsdom) | 30.1.3 | MIT |
| [openapi-typescript](https://www.npmjs.com/package/openapi-typescript) | 7.13.0 | MIT |
| [pg](https://www.npmjs.com/package/pg) | 8.23.1 | MIT |
| [prettier](https://www.npmjs.com/package/prettier) | 3.9.10 | MIT |
| [swagger2openapi](https://www.npmjs.com/package/swagger2openapi) | 7.0.8 | BSD-3-Clause |
| [typescript](https://www.npmjs.com/package/typescript) | 6.0.3 | Apache-2.0 |
| [typescript-eslint](https://www.npmjs.com/package/typescript-eslint) | 8.72.1 | MIT |
| [vite](https://www.npmjs.com/package/vite) | 8.3.5 | MIT |
| [vitest](https://www.npmjs.com/package/vitest) | 5.0.5 | MIT |
| [vue-tsc](https://www.npmjs.com/package/vue-tsc) | 3.3.14 | MIT |

---

## License Texts

### MIT License

The MIT License is used by the majority of dependencies. It is a permissive license that allows commercial use, modification, distribution, and private use.

Full text: https://opensource.org/licenses/MIT

### Apache License 2.0

Apache 2.0 is a permissive license similar to MIT but also provides an express grant of patent rights from contributors.

Full text: https://www.apache.org/licenses/LICENSE-2.0

### BSD Licenses (2-Clause and 3-Clause)

BSD licenses are permissive licenses similar to MIT. The 3-Clause variant includes an additional clause about using contributors' names for endorsement.

- BSD-2-Clause: https://opensource.org/licenses/BSD-2-Clause
- BSD-3-Clause: https://opensource.org/licenses/BSD-3-Clause

#### github.com/rickar/cal/v2 notice

The backend bundles this library's national holiday rules. Its BSD-3-Clause
notice is reproduced here and included in the runtime images:

```text
Copyright (c) 2014 Rick Arnold. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

 * Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.
 * Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.
 * Neither the name of Rick Arnold nor the names of any contributors may be
   used to endorse or promote products derived from this software without
   specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
POSSIBILITY OF SUCH DAMAGE.
```

### SIL Open Font License 1.1

The OFL lets fonts be used, bundled and redistributed with any software, provided the font files are not sold on their own.

Full text: https://openfontlicense.org/open-font-license-official-text/

### ISC License

ISC is functionally equivalent to MIT and BSD-2-Clause, but with simplified wording.

Full text: https://opensource.org/licenses/ISC

---

## Notes

1. **All licenses are BSL-compatible**: None of the dependencies use copyleft licenses (GPL, AGPL, LGPL) that would impose restrictions on WhenTo's BSL licensing.

2. **Date-holidays data**: The holiday calendar data in `date-holidays` is licensed under CC-BY-3.0 (Creative Commons Attribution), which applies to data rather than software. Attribution is provided through this document and the package's own license notices.

3. **Golang.org/x packages**: These are part of the Go standard library extensions and are licensed under the same BSD-3-Clause license as the Go programming language itself.

---

## Updating This Document

This document should be updated when:
- New dependencies are added
- Existing dependencies are upgraded to major versions
- Dependencies are removed

To regenerate the dependency list:

```bash
# Go dependencies — one module at a time; `./...` and `go list -m` never cross a
# module boundary, so `pkg/` has to be listed from inside it.
go list -m all
(cd pkg && go list -m all)

# NPM dependencies
cd frontend && npm list --all --depth=0
```

Licenses are read from the LICENSE file shipped in each module (`go env GOMODCACHE`)
and from the `license` field of each installed npm package — not from a package's
README or from memory.

---

**Last updated:** 2026-11-08
