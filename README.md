<p align="center">
  <img src="logo.png" alt="secha" width="112">
</p>

<p align="center">
  <strong>Measure whether your detection survives the removal of the control that masks it today.</strong>
</p>

<p align="center">
  <em>The control that stops the attack may be the only reason you'd catch it.<br>
  Measure what's left when it's gone.</em>
</p>

<p align="center">
  <a href="https://github.com/Thran34/secha/actions/workflows/ci.yml"><img src="https://github.com/Thran34/secha/actions/workflows/ci.yml/badge.svg" alt="ci"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-blue" alt="license Apache-2.0"></a>
  <a href="https://secha.dev"><img src="https://img.shields.io/badge/docs-secha.dev-informational" alt="secha.dev"></a>
</p>

---

`secha` is a command-line tool for Security Chaos Engineering on Kubernetes. It disables a security
control, runs the attack that control was supposed to stop, and measures whether anything else caught it.

The output is not a pass/fail. It is a set of numbers: was the data exposed, did anything detect the
attack, how long did detection take, did the cluster come back to the state it started in.

## Why another chaos tool

Chaos Mesh, LitmusChaos, Chaos Toolkit and Gremlin break **infrastructure** — they kill pods, saturate
CPU, drop packets, inject latency. They answer *does the system stay up*.

`secha` breaks **security controls** — a NetworkPolicy, an RBAC boundary, an admission policy. It answers
a different question: *when this control is gone, how much of the attack does the rest of the stack still
stop, and how fast does anything notice?*

That question needs three things the infrastructure tools do not provide:

| | |
|---|---|
| **A probe** | Removing a control proves nothing on its own. Something has to try the attack the control was blocking, or there is no signal to observe. |
| **A steady state that is verified** | If the target of the attack is not there to begin with, "nothing leaked" is a measurement artefact, not a defence. `secha` aborts before touching the cluster if the precondition is violated. |
| **A delta** | A single run is an anecdote. Resilience is the difference between two configurations under the same fault. |

## The loop

```
verify steady state  ──► abort if already violated (protects against a silent false negative)
        │
     inject fault    ──► disable or misconfigure a security control
        │
   reset detectors
        │
        probe        ──► run the attack that control was supposed to stop
        │
      observe        ──► did anything catch it, and after how long
        │
      restore        ──► always, including when the fault was rejected
        │
verify steady state  ──► recovery integrity: measured, not assumed
```

## Install

Requires Go 1.26+ and a kubeconfig pointing at the target cluster.

```sh
go install github.com/Thran34/secha/cmd/secha@latest
```

Or from a clone:

```sh
git clone https://github.com/Thran34/secha.git
cd secha
make build       # builds ./bin/secha
```

## Safety

`secha` deletes and modifies security controls on a live cluster. Three things keep that from becoming
an incident.

**It prints the cluster before it does anything.** Every run starts with the kubeconfig context and API
server it is about to act on. A kubeconfig whose current context drifted to the wrong cluster would
otherwise run the whole experiment somewhere unintended and report perfectly valid numbers.

**It restores the control even when interrupted.** Ctrl-C, `SIGTERM`, a failing probe, a fault that
half-applied before erroring — the control goes back. The restore runs on its own context, so a
cancellation cannot cancel the cleanup too. A second Ctrl-C aborts immediately and *will* leave the
control disabled; `secha` says so before you press it. If a restore ever does fail, the run ends with a
loud, unmissable error naming exactly what is still broken.

`SIGKILL` cannot be caught. Nothing recovers from `kill -9` mid-experiment.

**It can refuse to run on the wrong cluster.** Pin the experiment to the contexts it belongs to:

```yaml
safety:
  contexts: [secha-lab, staging]
```

Anything else is refused before the fault is injected. Without this block any cluster is accepted, and
`secha` says so on every run.

Use `--dry-run` to see exactly which control would be broken, without touching anything:

```sh
secha run examples/exp01-networkpolicy.yaml --dry-run
```

```
cluster: secha-lab (https://10.0.0.7:6443)

dry run, nothing was changed on secha-lab (https://10.0.0.7:6443)

  would break:  delete networkpolicy target/redis-allow-web-only
  would probe:  redis-exfil
  would watch:  redis-acl-log
```

## Try it in two minutes

`secha demo` needs nothing but a cluster you are allowed to break. It creates its own namespace,
measures one resilience delta, and deletes everything it created before it returns.

One requirement: the cluster must **enforce** NetworkPolicy, not merely accept the objects. Calico,
Cilium, kube-router and Weave all do. Minikube's default CNI does not — start it with
`minikube start --cni=calico`. If enforcement is missing, `secha` stops before injecting anything and
says so, rather than reporting a leak it did not cause.

```sh
secha demo
```

```
cluster: <your-context> (https://<your-api-server>)

secha demo creates the secha-demo namespace, measures, and deletes it again.

setting up the demo target
  waiting for the demo workloads to become ready

baseline:  demo-redis-without-auth (redis without AUTH)
treatment: demo-redis-with-auth (redis with AUTH)

resilience delta: demo-redis-without-auth → demo-redis-with-auth

  indicator           baseline     treatment     verdict
  fault applied       true         true          same
  reachable           true         true          —
  data exposed        true         false         improved
  detected            false        true          improved
  mttd                ∞ (never)    676.575041ms  improved
  recovery latency    17.552583ms  18.087459ms   —
  recovery integrity  true         true          same

what the AUTH layer was worth:

  without AUTH  the secret was read: true, anything detected it: false
  with AUTH     the secret was read: false, anything detected it: true

  Removing AUTH did not only cost the protection. It cost the detection too:
  the signal was the failed authentication, and with no AUTH there is none to log.
  A dashboard with no alerts would have looked exactly the same.

removing the secha-demo namespace
```

Two redis instances, identical except that one requires a password. The same NetworkPolicy is deleted
in front of both. The one with AUTH survives the attack **and** produces the log line that reveals it.
The one without AUTH loses the data and the visibility at the same time.

That second row is the point of the whole tool. A team that removed AUTH to simplify something would
have lost its detection without a single alert firing to say so.

## Quickstart

The `examples/` directory contains four ready experiments. `lab/` in the parent repository has the
manifests that create the target workloads they expect.

```sh
secha run examples/exp02-rbac.yaml
```

```
experiment: rbac-secret-read
  steady:  1 check(s)
  fault:   k8s-rbac
  probe:   k8s-secret-read
  observe: 1 detector(s), window 10s

report:
  fault applied:    false
    rejected by:    fault rejected by the cluster: roles.rbac.authorization.k8s.io "secha-escalation-default" is forbidden: ValidatingAdmissionPolicy 'secha-deny-secret-roles' with binding 'secha-deny-secret-roles' denied request: Role grants read access to secrets
  reachable:        true
  data exposed:     false
  detected:         false
  mttd:             ∞ (never)
  recovery latency: 23.145542ms
  recovery integrity: true
```

Read that carefully: the experiment did **not** fail. It measured that the cluster refused to let the
misconfiguration exist in the first place. `fault applied: false` is a result, and it is the strongest
one on the prevention axis.

## Commands

### `secha run <experiment.yaml>`

Runs one experiment and prints its indicators.

```sh
secha run examples/exp01-networkpolicy.yaml --repeat 10
secha run examples/exp01-networkpolicy.yaml --repeat 10 --output json > results.json
```

`--repeat N` runs the experiment N times and reports rates for booleans and mean / sd / min / max for
timings. With `--output json` the report goes to stdout and progress goes to stderr, so the pipe stays
clean.

### `secha compare <baseline.yaml> <treatment.yaml>`

Runs both and reports the per-indicator delta. This is where a resilience number actually comes from.

```sh
secha compare examples/exp02-rbac-unguarded.yaml examples/exp02-rbac.yaml
```

```
resilience delta: rbac-secret-read-unguarded → rbac-secret-read

  indicator           baseline     treatment    verdict
  fault applied       true         false        improved
  reachable           true         true         —
  data exposed        true         false        improved
  detected            true         n/a          —
  mttd                23.451375ms  n/a          —
  recovery latency    29.83875ms   36.602875ms  —
  recovery integrity  true         true         same
```

`detected` and `mttd` read `n/a` rather than `regressed`. Detection cannot be compared when the fault
never landed in the treatment — there was nothing to detect. `data exposed` stays comparable, because
not leaking is a real outcome regardless of which layer caused it.

### `secha demo`

Stands up a throwaway target, measures the AUTH delta shown above, and removes it again. No
configuration, no manifests to apply, nothing left behind. This is the fastest way to see the loop run.

### `secha init networkpolicy`

Writes an experiment for a NetworkPolicy you already run. It reads the policy from the cluster, works out
which service sits behind it and on which port, and produces a file you can run as-is.

```sh
secha init networkpolicy --namespace prod --policy redis-allow-web-only -o prod-redis.yaml
secha run prod-redis.yaml --dry-run
```

Nothing has to be deployed first. The generated experiment uses `k8s-tcp-connect`, which builds a
throwaway pod, tries the connection, and deletes the pod again — so the same file works on any cluster
that has the policy.

### `secha coverage <experiment.yaml>...`

Runs a set of experiments and aggregates the measured outcome per MITRE technique.

```sh
secha coverage examples/*.yaml --repeat 5 --navigator layer.json
```

```
ATT&CK techniques exercised:

  id         runs  exposed  detected  prevented  mttd (ms)  status    experiments
  T1098.006  2     50%      50%       50%        29.5       detected  rbac-secret-read, rbac-secret-read-unguarded
  T1552.007  2     50%      50%       50%        29.5       detected  rbac-secret-read, rbac-secret-read-unguarded

D3FEND countermeasures involved:

  id      runs  exposed  detected  prevented  mttd (ms)  status   experiments
  D3-ITF  1     0%       100%      0%         562.2      blocked  networkpolicy-web-to-db
```

`--navigator layer.json` writes an [ATT&CK Navigator](https://mitre-attack.github.io/attack-navigator/)
layer. Techniques are scored **100** when the attack was blocked, **50** when it succeeded but something
detected it, and **0** when it succeeded unnoticed.

The layer is filtered to the `Containers` platform by default so the matrix stays readable. Override with
`--platform Linux --platform Windows`, or pass `--platform ""` to disable the filter. `secha` prints which
platforms it filtered to, because a filter silently hides every technique outside it.

> Security products usually *declare* coverage of a technique because somebody wrote the identifier next
> to a feature. `secha` runs the attack, so it can *measure* it.

### `secha report <results.json>...`

Turns the JSON from `secha run` into figures. No plotting library, no browser toolchain — the SVG is
written by the same binary that took the measurements.

```sh
secha run examples/exp01-networkpolicy.yaml --repeat 8 --output json > exp01.json
secha run examples/exp02-rbac.yaml          --repeat 8 --output json > exp02.json

secha report exp01.json exp02.json -o report.html --svg-dir figures/
```

`-o` writes a self-contained HTML report — two charts and a table of every indicator, light and dark.
`--svg-dir` writes each figure as a standalone SVG for embedding in a document.

The two figures are chosen to carry the two questions the tool exists to answer:

- **Time to detect, per run** — one dot per run, the band spanning min to max, the mean ringed. An
  experiment that never detected says *never detected* rather than plotting at zero, which would read as
  instant detection.
- **Outcome rates across runs** — how often the fault landed, the data leaked, and something noticed.

## Experiment format

An experiment is a declarative YAML file. Every section maps to one plugin type.

```yaml
apiVersion: secha/v1
kind: Experiment
metadata:
  name: rbac-secret-read
  hypothesis: >
    A misconfigured RoleBinding grants the attacker ServiceAccount read access to
    the redis-auth secret. A posture check detects the privilege drift.

steadyState:
  checks:
    - type: k8s-secret-read
      as: {namespace: attacker, name: default}
      target: {namespace: target, secret: redis-auth}
      expect:
        dataExposed: false

fault:
  type: k8s-rbac
  attack: [T1098.006]
  action: grant
  subject: {namespace: attacker, name: default}
  grant:
    namespace: target
    resources: [secrets]
    verbs: [get, list]

probe:
  type: k8s-secret-read
  attack: [T1552.007]
  as: {namespace: attacker, name: default}
  target: {namespace: target, secret: redis-auth}

observe:
  window: 10s
  detectors:
    - type: k8s-access-review
      subject: {namespace: attacker, name: default}
      resource: {namespace: target, verb: get, resource: secrets, name: redis-auth}

indicators: [exposure, detection_coverage, mttd, recovery_latency]
```

A steady-state check is just a probe plus an `expect` block — no separate plugin type. An assertion left
out is not checked; an explicit `false` is checked and is not the same thing as absent.

`attack` and `d3fend` annotations may be attached to the fault, the probe, or any detector. Identifiers
are validated for shape when the experiment loads, so a typo does not travel into a coverage report.

## Indicators

| Indicator | What it measures |
|---|---|
| `fault applied` | Did the injected misconfiguration survive admission at all |
| `reachable` | Was the target reachable at the network layer |
| `data exposed` | Blast radius — did the attack actually get the data |
| `defense-in-depth depth` | How many layers had to fail before exposure |
| `detected` | Detection coverage — did any observer see it |
| `mttd` | Mean time to detect, over detected runs only |
| `recovery latency` | How long restoring the control took |
| `recovery integrity` | Was the steady state actually re-established afterwards |

`mttd` aggregates only over runs that detected something. Averaging in a non-detection as zero, or as the
observation window, would both be lies. Standard deviation uses the sample denominator (N−1).

## Built-in plugins

| Type | Kind | What it does |
|---|---|---|
| `k8s-networkpolicy` | fault | Deletes and restores a NetworkPolicy |
| `k8s-rbac` | fault | Grants an excess Role + RoleBinding, i.e. injects a misconfiguration |
| `redis-exfil` | probe | Reads a key from redis from an attacker pod |
| `k8s-secret-read` | probe | Reads a Secret as another ServiceAccount via impersonation |
| `k8s-tcp-connect` | probe | Opens a TCP connection from a throwaway pod it creates and deletes |
| `redis-key-present` | probe | Steady-state check that the attack target exists |
| `redis-acl-log` | detector | Polls the redis `ACL LOG` — action logging |
| `k8s-access-review` | detector | Polls `SubjectAccessReview` — posture / configuration drift |

The two detectors are deliberately different paradigms. Action logging is reactive: it needs the attack to
happen before there is anything to see. Posture checking is proactive: the drift exists the moment the
misconfiguration lands. On the same cluster they differ by roughly an order of magnitude in time to
detect.

## Extending

Three interfaces, in `internal/`:

```go
type Fault interface {
	Inject(ctx context.Context) error
	Restore(ctx context.Context) error
	Describe() string
}

type Probe interface {
	Run(ctx context.Context) (Result, error)
	Describe() string
}

type Detector interface {
	Observe(ctx context.Context, window time.Duration) (Signal, error)
	Describe() string
}
```

A detector may additionally implement `Resettable` to clear its state between runs, so a signal left over
from a previous experiment is not counted as a fresh detection.

Adding a control means one new file and one `Register("your-type", ctor)` call in its `init()`. The engine
does not change. A fault may report `fault.Rejected(detail)` when the cluster refuses the injection — that
is recorded as a measurement, not an error.

## Status

Working: the full canonical loop, two faults, two detection paradigms, repeated measurement with
aggregation, delta comparison, measured MITRE coverage with Navigator export, and figures rendered
straight from the measurements, a self-contained demo, and scaffolding for a policy you already run.
188 unit tests, none of which require a cluster.

Known limitations, stated rather than hidden:

- Observation is sequential, so `mttd` for a posture detector includes the probe duration. The drift
  exists the moment the fault lands; concurrent observation would report a smaller number.
- The `redis-acl-log` detector polls, which adds jitter on the order of the poll interval.
- Recovery integrity can be flaky for NetworkPolicy faults — the CNI needs a moment to enforce a
  re-created policy, and verification runs immediately.

Not implemented yet: runtime detection through Falco, Prometheus/Grafana as a signal source, faults for
mTLS and secrets management.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
