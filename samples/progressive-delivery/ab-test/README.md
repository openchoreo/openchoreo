# A/B test: recommendations in the cart

**Question:** is the new feature better? Each visitor stays in one group for the whole experiment, and one group gets the new version.

An Amazon engineer tested "customers who bought this also bought" in the cart against a vice president's orders, and it won by enough to ship ([Greg Linden](https://glinden.blogspot.com/2006/04/early-amazon-shopping-cart.html)). The storefront assigns each visitor a group once, 50/50, in the `exp_cartrecs` cookie, and the Endpoint routes on the cookie.

Run the commands from `samples/progressive-delivery/`.

## 1. Deploy and start the experiment

```sh
kubectl apply -f ab-test/components.yaml -f ab-test/endpoint.yaml
occ endpoint rollout start cart --strategy abTest --stable storefront \
  --candidate storefront-recs=storefront-recs/http --cookie exp_cartrecs=b
```

## 2. Each group sees its version

```sh
./tally.sh cart -b 'exp_cartrecs=b'    # storefront-recs
./tally.sh cart -b 'exp_cartrecs=a'    # storefront
```

An A/B test has no steps: group b gets the candidate until the test ends. To narrow it to phones, add a `User-Agent` header condition to the candidate's `match` next to the cookie.

## 3. Ship the winner

```sh
occ endpoint rollout finish cart --keep storefront-recs
```
