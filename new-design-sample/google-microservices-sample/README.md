Sure! Here are the instructions to build and run the Docker images for the Google Microservices Sample project, specifically for the `productcatalogservice`, `shippingservice`, `checkoutservice`, and `frontend` components.

Existing Docker images for these services are not available for ARM64 architecture, so you will need to build them from the source code.


```
git clone https://github.com/GoogleCloudPlatform/microservices-demo.git
cd microservices-demo/src/productcatalogservice
docker buildx build --platform=linux/arm64 -t productcatalogservice:arm64 .
kind load docker-image  productcatalogservice:arm64  --name openchoreo 
```


Same for shippingservice:
```
docker buildx build --platform=linux/arm64 -t shippingservice:arm64 .
kind load docker-image  shippingservice:arm64  --name openchoreo
```

Same for checkoutservice:
```
docker buildx build --platform=linux/arm64 -t checkoutservice:arm64 .
kind load docker-image  checkoutservice:arm64  --name openchoreo
```

same for frontend:
```
docker buildx build --platform=linux/arm64 -t frontend:arm64 .
kind load docker-image  frontend:arm64  --name openchoreo
```