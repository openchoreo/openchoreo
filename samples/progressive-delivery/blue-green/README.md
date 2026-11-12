# Blue/green: the EU VAT switch

**Question:** can every request change at the same moment? Preview the new version in production, then switch everyone at once, with blue still running for an instant switch back.

From 1 July 2021 an online shop selling across the EU charges VAT at the buyer's country rate, under the One-Stop Shop rules ([European Commission](https://vat-one-stop-shop.ec.europa.eu/one-stop-shop_en)). For a Dutch shop, a German buyer pays 19%, not the shop's own 21%. A canary would charge some buyers the old tax and some the new, so every checkout must change at once ([BlueGreenDeployment](https://martinfowler.com/bliki/BlueGreenDeployment.html)).

Run the commands from `samples/progressive-delivery/`. On the local k3d setup the Endpoint's URL is `http://development-checkout.openchoreoapis.localhost:19080/`.

## 1. Deploy both colors and plan the switch

```sh
kubectl apply -f blue-green/components.yaml -f blue-green/endpoint.yaml
occ endpoint rollout start checkout --strategy blueGreen --stable checkout-blue \
  --candidate checkout-green=checkout-green/http --header x-preview=green
```

## 2. Preview green

```sh
curl -s 'http://development-checkout.openchoreoapis.localhost:19080/?country=DE'                         # 21% (NL), blue
curl -s -H 'x-preview: green' 'http://development-checkout.openchoreoapis.localhost:19080/?country=DE'   # 19% (DE), green
```

## 3. Switch, and switch back

```sh
occ endpoint rollout step checkout --env development           # everyone on green
./tally.sh checkout
occ endpoint rollout step checkout --env development --back    # default traffic on blue again, in seconds; x-preview still reaches green
occ endpoint rollout step checkout --env development
```

The preview keeps working whichever way the switch points.

## 4. Retire blue

```sh
occ endpoint rollout finish checkout --keep checkout-green
```
