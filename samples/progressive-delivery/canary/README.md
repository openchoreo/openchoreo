# Canary: rewriting the trips service in Go

**Question:** is the new code safe? Give it a small share of real traffic, raise the share while it holds, and step back the moment it doesn't.

The trips service is written in Python and is slow, so the team rewrote it in Go: the same API, a new engine. Uber did this with the sharding layer of its Schemaless datastore, moving traffic over by percentage ([Uber](https://www.uber.com/blog/schemaless-rewrite/)). Google's SRE workbook calls it canarying ([Canarying releases](https://sre.google/workbook/canarying-releases/)).

Run the commands from `samples/progressive-delivery/`.

## 1. Deploy both versions

```sh
kubectl apply -f canary/components.yaml -f canary/endpoint.yaml
./tally.sh trips                 # every answer from trips-python
```

## 2. Plan the canary

```sh
occ endpoint rollout start trips --strategy canary --stable trips-python \
  --candidate trips-go=trips-go/http --steps 5,25,50,100 --header x-canary=true
occ endpoint rollout get trips
```

`trips-go` joins the Endpoint at 0%. Testers reach it already with the header, which works at any share:

```sh
./tally.sh trips -H 'x-canary: true'    # every answer from trips-go
```

The same plan in Git, for GitOps, with the backend the command added:

```yaml
spec:
  backendRefs:
    # ...the backends already there, trips-python among them
    - name: trips-go
      component: {name: trips-go, endpoint: http}
      weight: 0
      match:
        headers: [{name: x-canary, value: "true"}]
  rollout:
    strategy: canary
    stable: trips-python
    candidates: [{name: trips-go}]
    canary:
      steps:
        - backends: [{name: trips-go, percent: 5}]
        - backends: [{name: trips-go, percent: 25}]
        - backends: [{name: trips-go, percent: 50}]
        - backends: [{name: trips-go, percent: 100}]
```

## 3. Step through it

```sh
occ endpoint rollout step trips --env development      # 5%
occ endpoint rollout status trips --env development    # waits until the binding reports its routes applied and ready
./tally.sh trips                                       # about 5 of 100 from trips-go, both ways
occ endpoint rollout step trips --env development      # 25%
```

Something looks wrong? Go one step back, or straight back to step 0, where weighted traffic goes to stable (testers with the header still reach `trips-go`):

```sh
occ endpoint rollout step trips --env development --back
occ endpoint rollout abort trips --env development
```

In a pipeline, `--expect-step 1` refuses the step unless the environment is still at step 1, so a retried job can't skip a step. `--dry-run -o yaml` prints the binding change instead of making it, for a GitOps commit.

## 4. Finish

```sh
occ endpoint rollout step trips --env development --last    # 100%
occ endpoint rollout finish trips --keep trips-go
```

`finish` removes `trips-python` and the plan from the Endpoint, which cuts a release. It is refused while `trips-python` still gets traffic in any environment. In-cluster callers that still call `trips-python`'s own address reach it again once it leaves the Endpoint, so move them to `trips-go`'s address, then delete the `trips-python` component.

A new build of the same service doesn't need a second component: once release lines land, the candidate can be a release line of `trips` (`--candidate next=trips@next/http`).
