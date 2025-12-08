# Third-Party Licenses

This document lists all third-party dependencies used in WhenTo and their respective licenses.

WhenTo is licensed under the **Business Source License 1.1** (BSL). All dependencies listed below use permissive licenses that are fully compatible with BSL.

---

## License Compatibility Summary

✅ **All dependencies use BSL-compatible licenses:**

- **MIT License** - Majority of dependencies
- **Apache License 2.0** - Several Go and TypeScript packages
- **BSD-2-Clause / BSD-3-Clause** - PostgreSQL driver, Redis client, Google packages
- **ISC License** - Similar to MIT, fully permissive

❌ **No copyleft licenses** (GPL, AGPL, LGPL) are used in this project.

---

## Go Dependencies (Backend)

### Direct Dependencies

| Package | Version | License |
|---------|---------|---------|
| [github.com/arran4/golang-ical](https://github.com/arran4/golang-ical) | v0.3.2 | Apache-2.0 |
| [github.com/go-chi/chi](https://github.com/go-chi/chi) | v5.2.3 | MIT |
| [github.com/go-webauthn/webauthn](https://github.com/go-webauthn/webauthn) | v0.15.0 | BSD-3-Clause |
| [github.com/google/uuid](https://github.com/google/uuid) | v1.6.0 | BSD-3-Clause |
| [github.com/jackc/pgx](https://github.com/jackc/pgx) | v5.7.6 | MIT |
| [github.com/pquerna/otp](https://github.com/pquerna/otp) | v1.5.0 | Apache-2.0 |
| [github.com/skip2/go-qrcode](https://github.com/skip2/go-qrcode) | v0.0.0-20200617195104 | MIT |
| [github.com/spf13/cobra](https://github.com/spf13/cobra) | v1.10.2 | Apache-2.0 |
| [github.com/stripe/stripe-go](https://github.com/stripe/stripe-go) | v84.0.0 | MIT |
| [golang.org/x/crypto](https://golang.org/x/crypto) | v0.45.0 | BSD-3-Clause |

### Indirect Dependencies

| Package | Version | License |
|---------|---------|---------|
| github.com/boombuler/barcode | v1.0.1-0.20190219062509 | MIT |
| github.com/cespare/xxhash/v2 | v2.3.0 | MIT |
| github.com/dgryski/go-rendezvous | v0.0.0-20200823014737 | MIT |
| github.com/fxamacker/cbor/v2 | v2.9.0 | MIT |
| github.com/gabriel-vasile/mimetype | v1.4.11 | MIT |
| github.com/go-playground/locales | v0.14.1 | MIT |
| github.com/go-playground/tz | v0.0.1 | MIT |
| github.com/go-playground/universal-translator | v0.18.1 | MIT |
| github.com/go-playground/validator/v10 | v10.28.0 | MIT |
| github.com/go-viper/mapstructure/v2 | v2.4.0 | MIT |
| github.com/go-webauthn/x | v0.1.26 | BSD-3-Clause |
| github.com/golang-jwt/jwt/v5 | v5.3.0 | MIT |
| github.com/google/go-cmp | v0.6.0 | BSD-3-Clause |
| github.com/google/go-tpm | v0.9.6 | BSD-3-Clause |
| github.com/google/go-tpm-tools | v0.3.13-0.20230620182252 | BSD-3-Clause |
| github.com/inconshreveable/mousetrap | v1.1.0 | Apache-2.0 |
| github.com/jackc/pgpassfile | v1.0.0 | MIT |
| github.com/jackc/pgservicefile | v0.0.0-20240606120523 | MIT |
| github.com/jackc/puddle/v2 | v2.2.2 | MIT |
| github.com/leodido/go-urn | v1.4.0 | MIT |
| github.com/omidnikrah/go-holidays | v1.0.0 | MIT |
| github.com/redis/go-redis/v9 | v9.17.2 | BSD-2-Clause |
| github.com/spf13/pflag | v1.0.10 | BSD-3-Clause |
| github.com/stretchr/testify | v1.11.1 | MIT |
| github.com/x448/float16 | v0.8.4 | MIT |
| go.uber.org/mock | v0.6.0 | Apache-2.0 |
| golang.org/x/mod | v0.29.0 | BSD-3-Clause |
| golang.org/x/net | v0.47.0 | BSD-3-Clause |
| golang.org/x/sync | v0.18.0 | BSD-3-Clause |
| golang.org/x/sys | v0.38.0 | BSD-3-Clause |
| golang.org/x/term | v0.37.0 | BSD-3-Clause |
| golang.org/x/text | v0.31.0 | BSD-3-Clause |
| golang.org/x/tools | v0.38.0 | BSD-3-Clause |
| gopkg.in/yaml.v3 | v3.0.1 | MIT |

---

## NPM Dependencies (Frontend)

### Production Dependencies

| Package | Version | License |
|---------|---------|---------|
| [axios](https://www.npmjs.com/package/axios) | 1.13.2 | MIT |
| [countries-and-timezones](https://www.npmjs.com/package/countries-and-timezones) | 3.8.0 | MIT |
| [date-fns-tz](https://www.npmjs.com/package/date-fns-tz) | 3.2.0 | MIT |
| [date-holidays](https://www.npmjs.com/package/date-holidays) | 3.26.5 | ISC* |
| [i18n-iso-countries](https://www.npmjs.com/package/i18n-iso-countries) | 7.14.0 | MIT |
| [vue](https://www.npmjs.com/package/vue) | 3.5.25 | MIT |
| [vue-router](https://www.npmjs.com/package/vue-router) | 4.6.3 | MIT |
| [world-countries](https://www.npmjs.com/package/world-countries) | 5.1.0 | ODbL |

\* *date-holidays: Code is ISC licensed, holiday data is CC-BY-3.0 (Creative Commons Attribution)*

### Development Dependencies

| Package | Version | License |
|---------|---------|---------|
| [@tailwindcss/postcss](https://www.npmjs.com/package/@tailwindcss/postcss) | 4.1.17 | MIT |
| [@types/node](https://www.npmjs.com/package/@types/node) | 24.10.1 | MIT |
| [@typescript-eslint/eslint-plugin](https://www.npmjs.com/package/@typescript-eslint/eslint-plugin) | 8.48.1 | MIT |
| [@typescript-eslint/parser](https://www.npmjs.com/package/@typescript-eslint/parser) | 8.48.1 | BSD-2-Clause |
| [@vitejs/plugin-vue](https://www.npmjs.com/package/@vitejs/plugin-vue) | 6.0.2 | MIT |
| [@vueuse/core](https://www.npmjs.com/package/@vueuse/core) | 14.1.0 | MIT |
| [autoprefixer](https://www.npmjs.com/package/autoprefixer) | 10.4.22 | MIT |
| [date-fns](https://www.npmjs.com/package/date-fns) | 4.1.0 | MIT |
| [eslint](https://www.npmjs.com/package/eslint) | 9.39.1 | MIT |
| [eslint-plugin-vue](https://www.npmjs.com/package/eslint-plugin-vue) | 10.6.2 | MIT |
| [pinia](https://www.npmjs.com/package/pinia) | 3.0.4 | MIT |
| [postcss](https://www.npmjs.com/package/postcss) | 8.5.6 | MIT |
| [prettier](https://www.npmjs.com/package/prettier) | 3.7.4 | MIT |
| [tailwindcss](https://www.npmjs.com/package/tailwindcss) | 4.1.17 | MIT |
| [typescript](https://www.npmjs.com/package/typescript) | 5.9.3 | Apache-2.0 |
| [vite](https://www.npmjs.com/package/vite) | 7.2.6 | MIT |
| [vue-i18n](https://www.npmjs.com/package/vue-i18n) | 11.2.2 | MIT |
| [vue-tsc](https://www.npmjs.com/package/vue-tsc) | 3.1.6 | MIT |

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
# Go dependencies
go list -m all

# NPM dependencies
cd frontend && npm list --all --depth=0
```

---

**Last updated:** 2025-12-07