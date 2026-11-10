# Dun — Guide de compréhension du projet

> Document de synthèse (en français) : ce qu'est Dun, où il en est,
> à quoi il sert et à quoi il ne sert pas. Pour la doc technique
> complète, voir `README.md`, `docs/ARCHITECTURE.md`,
> `docs/EXTENSIONS.md`, `docs/BENCHMARKS.md` et `rfcs/`.

## 1. Dun c'est quoi ?

**Un proxy HTTP programmable en Go, dont la logique métier tourne dans
des plugins WebAssembly sandboxés.**

L'analogie la plus parlante : c'est un mini-Envoy/Traefik écrit en pur
Go, où chaque "filter" est un module Wasm isolé. La différence
fondamentale avec Envoy : le cœur est *minuscule* et l'extensibilité
passe par un sandbox Wasm (via **wazero**, runtime Wasm 100% Go, sans
cgo), pas par un gros framework C++/Go.

```
client → Dun (routeur + proxy) → pipeline d'extensions Wasm → upstream
```

## 2. Architecture en 5 couches

| Couche | Package | Rôle |
|---|---|---|
| CLI | `cmd/dun` | `run`, `check`, `version`, `plugin validate/inspect` |
| Config | `internal/config` | YAML strict (`KnownFields`), validation |
| Serveur | `internal/http` | `ServeHTTP` : healthz/readyz/metrics, routing, pipeline, proxy |
| Routage | `internal/router` | exact / prefix / host / method, first-match |
| Proxy | `internal/proxy` | `httputil.ReverseProxy` par upstream nommé |
| Pipeline | `internal/pipeline` | extensions ordonnées, décisions CONTINUE/RESPOND/REJECT/ERROR |
| Extensions | `internal/extension` | manager, lifecycle, capabilities, télémétrie |
| Runtime | `internal/runtime/wazero` | hôte Wasm : compile, instancie, host functions |
| Guests | `internal/guest` | modules Wasm assemblés à la main (tests/exemples) |

## 3. Le modèle d'extension (le cœur du projet)

### ABI Core-Wasm v0

Un plugin Wasm doit exporter :

- `dun_abi_version_0_1() -> i32` : marqueur d'ABI (retourne 1), vérifié à
  chaque load/reload ;
- `dun_process_request() -> i32` : décision `0=CONTINUE, 1=RESPOND,
  2=REJECT, 3=ERROR`.

### Capabilities (deux étages)

Le plugin ne voit que ce que la config lui accorde :

- **Load-time** : les capabilities déclarées sont vérifiées contre la
  policy → refus rapide ;
- **Runtime** : seules les host functions accordées sont exposées au
  guest — importer une fonction non accordée fait échouer
  l'instantiation (refus *authoritatif*).

Host functions v0 actuelles :

| Import | Rôle |
|---|---|
| `request.headers.read` | lit un header de requête (ajouté récemment) |
| `response.headers.write` | écrit un header de réponse |
| `log.info` / `log.error` | logging depuis le guest |

### Cycle de vie

`DISCOVER → VALIDATE → LOAD → COMPILE → INSTANTIATE → INITIALIZE →
READY → SERVING → DRAINING → CLOSED`, avec hot reload **atomique** : le
nouveau binaire est compilé pendant que l'ancien sert, puis swap en une
opération. Déclenché par un watcher de fichiers (polling 5s).

### Isolation

- **Par défaut (§15.1)** : instance Wasm **fraîche par requête** —
  isolation maximale, zéro état partagé ;
- **Opt-in (§15.2, `limits.instances: N`)** : pool d'instances recyclées
  avec restauration mémoire par snapshot — ~4-5× plus rapide, ~7× moins
  d'allocations (voir `docs/BENCHMARKS.md`). Réservé aux artefacts
  "pool-safe" (pas de tables/globals/elem).

### Limites sandbox

`limits.memory` (linéaire), `limits.timeout` (deadline par invocation →
500), échecs classifiés (`trap/timeout/cancelled/instantiate/host_call`)
enregistrés en télémétrie **et exposés sur `/metrics`**.

## 4. Où il en est (état réel au 2026-11)

| Milestone | État |
|---|---|
| M1 proxy natif + routing | ✅ |
| M2 runtime wazero | ✅ |
| M3 contrat d'extension + plugin header | ✅ |
| M4 plugins multiples / pipelines par route | ✅ |
| M5 isolation + limites | ✅ |
| M6 hot reload atomique | ✅ |
| M7 Benchmark Lab + pooling | ✅ (ce chantier) |
| M8 recherche écosystème (Proxy-Wasm, Components, WIT) | ⬜ |

**Chiffres clés** (machine dev, voir `docs/BENCHMARKS.md`) :

- invoke Wasm poolé : **~25µs, 21 allocs** ; per-request : ~110µs, 48 allocs ;
- chemin HTTP complet dominé par le proxy (~0.5-1ms/boucle locale) ;
- sous parallélisme : poolé ~29% plus rapide et ~52% moins de mémoire ;
- reload plugin : ~2.5ms ; compile plugin : ~1.2ms.

## 5. À quoi Dun servirait (cas d'usage réels)

Là où un plugin Wasm est *pertinent* :

- **AuthN/Z légère par headers** : le plugin `HeaderReadModule` d'exemple
  rejette déjà les requêtes avec `X-Denied` — un plugin peut faire du
  allow/deny sur n'importe quel header (API keys, feature flags,
  maintenance mode) sans toucher au cœur ;
- **Security headers / audit** : ajouter des headers de réponse
  (CSP, HSTS…) par route ;
- **Logging métier** : `log.info` depuis le guest → observabilité
  par-plugin sur `/metrics` ;
- **Sandbox d'expérimentation** : prototyper un comportement de gateway
  en Wasm sans recompiler ni risquer le cœur — un plugin qui trap ne
  coûte qu'un 500, le cœur survit ;
- **Apprentissage/portfolio** : c'est un projet d'architecture
  infrastructure sérieux et mesuré — les RFCs et benchmarks le
  rendent crédible techniquement.

## 6. Où Dun ne faut PAS l'utiliser

Soyons honnêtes — la spec elle-même le dit (§34 Non-Goals) :

- **Pas de remplacement d'Envoy/Traefik/Kong en production** : pas de
  service discovery, pas de LB, pas de control plane, pas de streaming
  body, pas de mTLS avancé, pas de signatures de plugins ;
- **Le pool-safe gate limite les guests complexes** : un plugin avec
  globals/tables ne peut pas être poolé ;
- **Body streaming absent** (RFC-0004) : impossible de transformer le
  corps de requête/réponse aujourd'hui ;
- **Un seul fichier de config YAML** : pas de reload de config, pas de
  découverte dynamique.

## 7. Feuille de route (ce qui manque pour "vrai" projet)

Par ordre de valeur probable :

1. ~~Benchmark Lab + décision pooling~~ ✅ fait ;
2. ~~`request.headers.read`~~ ✅ fait — les plugins peuvent enfin *lire* ;
3. **Body read/write borné** (`request.body.read`, `max_body_size`
   déjà parsé) — débloque auth sur body, transforms simples ;
4. **`response.status.write` + body de réponse** — rendre la décision
   `RESPOND` réellement utile (réponses synthétiques : rate-limit 429,
   captcha, maintenance) ;
5. **Streaming** (RFC-0004) — le vrai défi architectural ;
6. **RFCs manquants** : capability model (0002), lifecycle (0003),
   streaming (0004), Proxy-Wasm (0006), Components (0007), signing (0008) ;
7. **Gen-2 Component Model** — la vision long terme (WIT, async).

## 8. Comment s'en servir concrètement

```bash
# démarrer l'exemple gateway complet (backend :3000 + dun :8080 + plugin)
go run ./examples/gateway/backend &
go run ./cmd/dun run -config examples/gateway/dun.yaml
curl -i http://localhost:8080/api/hello

# valider / inspecter des plugins
go run ./cmd/dun check -config examples/gateway/dun.yaml
go run ./cmd/dun plugin inspect -config examples/gateway/dun.yaml

# tests complets
go test ./... && go test -race ./...

# benchmarks
go test ./benchmarks -bench . -benchmem -benchtime=2s
```

Plugin minimal : prendre `plugins/examples/security-header/` comme
modèle (WAT dans `docs/EXTENSIONS.md`, ou SDK TinyGo/Rust dans `sdk/`).

## 9. En une phrase

> Dun est un proxy HTTP en Go volontairement petit, dont chaque
> comportement est une extension WebAssembly sandboxée, à
> capabilities, hot-reloadable et mesurée — un banc d'essai sérieux
> pour la question "Wasm comme frontière d'extension en infrastructure
> HTTP", avec les benchmarks pour le prouver.
