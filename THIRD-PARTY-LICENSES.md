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
| [github.com/go-chi/chi/v5](https://github.com/go-chi/chi) | v5.3.1 | MIT |
| [github.com/go-playground/tz](https://github.com/go-playground/tz) | v0.0.1 | MIT |
| [github.com/go-playground/validator/v10](https://github.com/go-playground/validator) | v10.30.3 | MIT |
| [github.com/go-webauthn/webauthn](https://github.com/go-webauthn/webauthn) | v0.17.4 | BSD-3-Clause |
| [github.com/golang-jwt/jwt/v5](https://github.com/golang-jwt/jwt) | v5.3.1 | MIT |
| [github.com/google/uuid](https://github.com/google/uuid) | v1.6.0 | BSD-3-Clause |
| [github.com/jackc/pgx/v5](https://github.com/jackc/pgx) | v5.10.0 | MIT |
| [github.com/joho/godotenv](https://github.com/joho/godotenv) | v1.5.1 | MIT |
| [github.com/omidnikrah/go-holidays](https://github.com/omidnikrah/go-holidays) | v1.0.0 | MIT |
| [github.com/pquerna/otp](https://github.com/pquerna/otp) | v1.5.0 | Apache-2.0 |
| [github.com/prometheus/client_golang](https://github.com/prometheus/client_golang) | v1.24.1 | Apache-2.0 |
| [github.com/redis/go-redis/v9](https://github.com/redis/go-redis) | v9.22.0 | BSD-2-Clause |
| [github.com/skip2/go-qrcode](https://github.com/skip2/go-qrcode) | v0.0.0-20200617195104 | MIT |
| [github.com/swaggo/http-swagger/v2](https://github.com/swaggo/http-swagger) | v2.0.2 | MIT |
| [github.com/swaggo/swag](https://github.com/swaggo/swag) | v1.16.6 | MIT |
| [golang.org/x/crypto](https://golang.org/x/crypto) | v0.55.0 | BSD-3-Clause |
| [golang.org/x/time](https://golang.org/x/time) | v0.15.0 | BSD-3-Clause |

### Indirect Dependencies

| Package | Version | License |
|---------|---------|---------|
| github.com/KyleBanks/depth | v1.2.1 | MIT |
| github.com/beorn7/perks | v1.0.1 | MIT |
| github.com/boombuler/barcode | v1.1.0 | MIT |
| github.com/cespare/xxhash/v2 | v2.3.0 | MIT |
| github.com/fxamacker/cbor/v2 | v2.9.2 | MIT |
| github.com/gabriel-vasile/mimetype | v1.4.15 | MIT |
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
| github.com/go-playground/locales | v0.14.1 | MIT |
| github.com/go-playground/universal-translator | v0.18.1 | MIT |
| github.com/go-viper/mapstructure/v2 | v2.5.0 | MIT |
| github.com/go-webauthn/x | v0.2.8 | BSD-3-Clause |
| github.com/google/go-tpm | v0.9.8 | Apache-2.0 |
| github.com/jackc/pgpassfile | v1.0.0 | MIT |
| github.com/jackc/pgservicefile | v0.0.0-20240606120523 | MIT |
| github.com/jackc/puddle/v2 | v2.2.2 | MIT |
| github.com/kylelemons/godebug | v1.1.0 | Apache-2.0 |
| github.com/leodido/go-urn | v1.5.0 | MIT |
| github.com/munnerz/goautoneg | v0.0.0-20191010083416 | BSD-3-Clause |
| github.com/philhofer/fwd | v1.2.0 | MIT |
| github.com/prometheus/client_model | v0.6.2 | Apache-2.0 |
| github.com/prometheus/common | v0.70.1 | Apache-2.0 |
| github.com/prometheus/procfs | v0.21.1 | Apache-2.0 |
| github.com/swaggo/files/v2 | v2.0.2 | MIT |
| github.com/tinylib/msgp | v1.6.4 | MIT |
| github.com/x448/float16 | v0.8.4 | MIT |
| go.uber.org/atomic | v1.11.0 | MIT |
| go.yaml.in/yaml/v3 | v3.0.5 | MIT AND Apache-2.0* |
| golang.org/x/mod | v0.39.0 | BSD-3-Clause |
| golang.org/x/sync | v0.22.0 | BSD-3-Clause |
| golang.org/x/sys | v0.47.0 | BSD-3-Clause |
| golang.org/x/text | v0.41.0 | BSD-3-Clause |
| golang.org/x/tools | v0.48.0 | BSD-3-Clause |
| google.golang.org/protobuf | v1.36.11 | BSD-3-Clause |

\* *go.yaml.in/yaml/v3 ships a single LICENSE covering the package under both MIT and Apache-2.0; both are permissive and BSL-compatible.*

---

## NPM Dependencies (Frontend)

### Production Dependencies

| Package | Version | License |
|---------|---------|---------|
| [@vueuse/core](https://www.npmjs.com/package/@vueuse/core) | 14.4.0 | MIT |
| [axios](https://www.npmjs.com/package/axios) | 1.19.0 | MIT |
| [countries-and-timezones](https://www.npmjs.com/package/countries-and-timezones) | 3.10.0 | MIT |
| [date-fns](https://www.npmjs.com/package/date-fns) | 4.4.0 | MIT |
| [date-fns-tz](https://www.npmjs.com/package/date-fns-tz) | 3.2.0 | MIT |
| [date-holidays](https://www.npmjs.com/package/date-holidays) | 3.34.1 | ISC* |
| [i18n-iso-countries](https://www.npmjs.com/package/i18n-iso-countries) | 7.14.0 | MIT |
| [pinia](https://www.npmjs.com/package/pinia) | 4.0.3 | MIT |
| [vue](https://www.npmjs.com/package/vue) | 3.5.41 | MIT |
| [vue-i18n](https://www.npmjs.com/package/vue-i18n) | 11.4.8 | MIT |
| [vue-router](https://www.npmjs.com/package/vue-router) | 5.2.0 | MIT |
| [world-countries](https://www.npmjs.com/package/world-countries) | 5.1.0 | ODbL-1.0 |

\* *date-holidays: Code is ISC licensed, holiday data is CC-BY-3.0 (Creative Commons Attribution)*

### Development Dependencies

| Package | Version | License |
|---------|---------|---------|
| [@eslint/js](https://www.npmjs.com/package/@eslint/js) | 10.0.1 | MIT |
| [@playwright/test](https://www.npmjs.com/package/@playwright/test) | 1.62.1 | Apache-2.0 |
| [@tailwindcss/postcss](https://www.npmjs.com/package/@tailwindcss/postcss) | 4.3.3 | MIT |
| [@types/node](https://www.npmjs.com/package/@types/node) | 24.13.3 | MIT |
| [@vitejs/plugin-vue](https://www.npmjs.com/package/@vitejs/plugin-vue) | 6.0.8 | MIT |
| [@vitest/coverage-v8](https://www.npmjs.com/package/@vitest/coverage-v8) | 4.1.10 | MIT |
| [eslint](https://www.npmjs.com/package/eslint) | 10.8.1 | MIT |
| [eslint-config-prettier](https://www.npmjs.com/package/eslint-config-prettier) | 10.1.8 | MIT |
| [eslint-plugin-vue](https://www.npmjs.com/package/eslint-plugin-vue) | 10.10.0 | MIT |
| [eslint-plugin-vuejs-accessibility](https://www.npmjs.com/package/eslint-plugin-vuejs-accessibility) | 2.6.0 | MIT |
| [globals](https://www.npmjs.com/package/globals) | 17.11.0 | MIT |
| [jsdom](https://www.npmjs.com/package/jsdom) | 30.0.1 | MIT |
| [prettier](https://www.npmjs.com/package/prettier) | 3.9.6 | MIT |
| [typescript](https://www.npmjs.com/package/typescript) | 6.0.3 | Apache-2.0 |
| [typescript-eslint](https://www.npmjs.com/package/typescript-eslint) | 8.67.0 | MIT |
| [vite](https://www.npmjs.com/package/vite) | 8.2.1 | MIT |
| [vitest](https://www.npmjs.com/package/vitest) | 4.1.10 | MIT |
| [vue-tsc](https://www.npmjs.com/package/vue-tsc) | 3.3.9 | MIT |

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

### ISC License

ISC is functionally equivalent to MIT and BSD-2-Clause, but with simplified wording.

Full text: https://opensource.org/licenses/ISC

---

## Notes

1. **All licenses are BSL-compatible**: None of the dependencies use copyleft licenses (GPL, AGPL, LGPL) that would impose restrictions on WhenTo's BSL licensing.

2. **Date-holidays data**: The holiday calendar data in `date-holidays` is licensed under CC-BY-3.0 (Creative Commons Attribution), which applies to data rather than software. Attribution is provided through this document and the package's own license notices.

3. **World-countries data**: Licensed under ODbL (Open Database License), which applies to the geographical data. Usage complies with ODbL requirements through attribution.

4. **Golang.org/x packages**: These are part of the Go standard library extensions and are licensed under the same BSD-3-Clause license as the Go programming language itself.

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

**Last updated:** 2026-08-12