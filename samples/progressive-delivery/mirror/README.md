# Mirror: shadowing the trips rewrite

**Question:** does the new code survive real traffic before anyone depends on it? Copy a share of requests to it and drop its answers, so every caller still gets the current version.

This runs before the [canary](../canary/): the Go rewrite of the trips service sees production traffic while every rider still gets Python. Google's SRE book calls it dark launching ([Reliable product launches](https://sre.google/sre-book/reliable-product-launches/)).

Run the commands from `samples/progressive-delivery/`.

## 1. Deploy and plan the mirror

```sh
kubectl apply -f canary/components.yaml -f canary/endpoint.yaml
occ endpoint rollout start trips --strategy mirror --stable trips-python \
  --candidate trips-go=trips-go/http --steps 10,100
```

## 2. Copy 10%, then everything

```sh
occ endpoint rollout step trips --env development       # copies 10%
./tally.sh trips --copies trips-go
```

Every answer comes from `trips-python`. `trips-go` has no callers of its own, so every request it serves is a copy: about 20 of the 200 the tally sends.

```sh
occ endpoint rollout step trips --env development       # copies 100%
./tally.sh trips --copies trips-go
```

## 3. Finish, then canary

```sh
occ endpoint rollout finish trips --keep trips-python
```

`finish` removes the shadow and the plan. Carry on with the [canary](../canary/README.md#2-plan-the-canary) to hand `trips-go` real callers.
