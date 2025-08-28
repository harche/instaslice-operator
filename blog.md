# Why Run One AI Model When You Can Run Ten? Slicing GPUs on OpenShift

Your GPU has a split personality. On Monday morning it idles while a tiny service waits for requests; by lunch it’s pegged at 100% serving a single chunky model. What if the same GPU could flex between those extremes: running seven bite‑size models before noon and then a full GPU workload after? That’s the promise of NVIDIA MIG paired with OpenShift’s Dynamic Accelerator Slicer (DAS) Operator.

In this post, we’ll take a tour of that world. We’ll start with a quick, human‑friendly explanation of MIG, show how DAS turns “GPU partitions” into just‑in‑time, Kubernetes‑native resources, and then spin up three live demos: from seven tiny models on one card, to two medium models, to a single full GPU workload. Along the way you’ll see how to keep GPUs busy, teams isolated, and operations simple.

## MIG, Explained Without the Jargon

Think of a large GPU as a high‑rise. MIG (Multi‑Instance GPU) lets you split that building into separate apartments with walls, doors, and their own utilities. A100 40GB cards, for example, can become:

- 1g.5gb apartments for tiny workloads (you can fit seven)
- 3g.20gb apartments for mid‑sized models
- 7g.40gb a full‑floor penthouse when one large tenant needs everything

Each apartment is isolated, with no noisy neighbors, and performance is predictable. The result: better utilization and safer multi‑tenancy without the “who stole my GPU?” drama.

## Why Do It Dynamically?

Static partitions go stale. Teams change, workloads spike, and idle slices collect dust. The DAS Operator makes slicing ephemeral: your pod asks for a slice, the operator creates it right before the container starts, and removes it when the pod goes away. No SSH, no pets, no hand‑carved layouts; just standard Kubernetes scheduling with right‑sized GPU resources.

![DAS Architecture](docs/images/arch.png)

## What You’ll Need

You’re on OpenShift with nodes that support NVIDIA MIG, plus Node Feature Discovery and the NVIDIA GPU Operator installed. MIG should be enabled on your GPU nodes per the GPU Operator docs. That’s it. No bespoke scripts or cluster snowflakes.

## Install the DAS Operator

Use the OpenShift Web Console to install cert‑manager, then the NVIDIA GPU Operator and Node Feature Discovery, and finally the Dynamic Accelerator Slicer Operator. Create a `DASOperator` instance with defaults (emulation off) and wait for the operator pods to go green. For reference and deeper guidance:

OpenShift docs: Dynamic Accelerator Slicer Operator (installation and setup)
https://97359--ocpdocs-pr.netlify.app/openshift-enterprise/latest/hardware_accelerators/das-about-dynamic-accelerator-slicer-operator

## One‑Time Setup: Hugging Face Token

Our examples pull models from Hugging Face. Create a secret named `huggingface-secret` with key `HF_TOKEN` in the `default` namespace once, and reuse it everywhere:

```bash
# Replace <your_hf_token>
kubectl create secret generic huggingface-secret \
  --from-literal=HF_TOKEN=<base64 ofyour hf_token>

# Verify it exists
kubectl get secret huggingface-secret
```


## A Day in the Life of a GPU: Three Demos

We’ll show the entire spectrum, from many tiny models to one full GPU workload, on a single cluster. Each example uses vLLM for serving and requests a different MIG profile. Watch how the same GPU card shape‑shifts to match what you deploy.

### Demo 1: Seven Tiny Models, One Card

Sometimes throughput beats raw horsepower. Here we run seven replicas of Gemma 3 270M, each requesting `nvidia.com/mig-1g.5gb`. OpenShift co‑schedules them on the same node; DAS carves seven 1g.5gb slices right on time.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vllm-gemma-270m
  labels:
    app: vllm-gemma-270m
spec:
  replicas: 7
  selector:
    matchLabels:
      app: vllm-gemma-270m
  template:
    metadata:
      labels:
        app: vllm-gemma-270m
    spec:
      restartPolicy: Always
      affinity:
        podAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels:
                  app: vllm-gemma-270m
              topologyKey: kubernetes.io/hostname
      containers:
      - name: vllm
        image: vllm/vllm-openai:latest
        ports:
        - containerPort: 8003
        env:
        - name: HUGGING_FACE_HUB_TOKEN
          valueFrom:
            secretKeyRef:
              name: huggingface-secret
              key: HF_TOKEN
        command: ["/bin/bash"]
        args:
        - -c
        - |
          # Start vLLM server with Gemma 3 270M model
          # Fixed port so a single Service can target all replicas
          echo "Starting vLLM on port 8003 for pod $HOSTNAME"
          vllm serve google/gemma-3-270m --host 0.0.0.0 --port 8003 --max-model-len 1024 --gpu-memory-utilization 0.7
        resources:
          limits:
            nvidia.com/mig-1g.5gb: 1  # Dynamically created by DAS
          requests:
            memory: "2Gi"
            cpu: "0.5"
---
apiVersion: v1
kind: Service
metadata:
  name: vllm-gemma-270m
spec:
  selector:
    app: vllm-gemma-270m
  ports:
  - port: 8003
    targetPort: 8003
    name: http
```

Deploy and watch them land on the same node:

```bash
kubectl apply -f gemma.yaml
kubectl get pods -l app=vllm-gemma-270m -o wide
```

Actual output on the cluster:

```
NAME                               READY   STATUS    RESTARTS   AGE   IP            NODE                                     NOMINATED NODE   READINESS GATES
vllm-gemma-270m-5d866f98db-2q6qv   1/1     Running   0          71m   10.129.2.56   harpatil000043jma-p5jcx-worker-f-7r4b8   <none>           <none>
vllm-gemma-270m-5d866f98db-8hxj4   1/1     Running   0          71m   10.129.2.53   harpatil000043jma-p5jcx-worker-f-7r4b8   <none>           <none>
vllm-gemma-270m-5d866f98db-b5rmx   1/1     Running   0          71m   10.129.2.59   harpatil000043jma-p5jcx-worker-f-7r4b8   <none>           <none>
vllm-gemma-270m-5d866f98db-m28kx   1/1     Running   0          71m   10.129.2.55   harpatil000043jma-p5jcx-worker-f-7r4b8   <none>           <none>
vllm-gemma-270m-5d866f98db-pbmhf   1/1     Running   0          71m   10.129.2.54   harpatil000043jma-p5jcx-worker-f-7r4b8   <none>           <none>
vllm-gemma-270m-5d866f98db-vqg9p   1/1     Running   0          71m   10.129.2.58   harpatil000043jma-p5jcx-worker-f-7r4b8   <none>           <none>
vllm-gemma-270m-5d866f98db-w8j7b   1/1     Running   0          71m   10.129.2.57   harpatil000043jma-p5jcx-worker-f-7r4b8   <none>           <none>
```

Quick smoke‑test one replica via the service:

```bash
kubectl port-forward svc/vllm-gemma-270m 8003:8003 &
curl -X POST http://localhost:8003/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "google/gemma-3-270m",
    "prompt": "The capital of France is ",
    "max_tokens": 80,
    "temperature": 0.7
  }'
```

Sample Response:

```json
{
  "id": "cmpl-52bf37b81aed469986d69c9a866706c9",
  "object": "text_completion",
  "created": 1756411573,
  "model": "google/gemma-3-270m",
  "choices": [
    {
      "index": 0,
      "text": "<strong>Paris</strong>. It’s also the most expensive city in the world, with a price tag of <strong>$100,000,000</strong>.\n\nParis is the <strong>fifth most expensive city in the world</strong>, and it’s on the list of 100 most expensive cities in the world.\n\n<strong>Paris</strong> is the world’s",
      "logprobs": null,
      "finish_reason": "length",
      "stop_reason": null,
      "prompt_logprobs": null
    }
  ],
  "service_tier": null,
  "system_fingerprint": null,
  "usage": {
    "prompt_tokens": 7,
    "total_tokens": 87,
    "completion_tokens": 80,
    "prompt_tokens_details": null
  },
  "kv_transfer_params": null
}
```

Result: seven isolated services sharing one GPU cleanly, each with its own slice and predictable performance.

### Demo 2: Two Qwen2 7B Instruct Models

Now the middle ground: two strong instruct models side‑by‑side. Each requests a `3g.20gb` slice for a balanced split of the card.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vllm-qwen-7b
  labels:
    app: vllm-qwen-7b
spec:
  replicas: 2
  selector:
    matchLabels:
      app: vllm-qwen-7b
  template:
    metadata:
      labels:
        app: vllm-qwen-7b
    spec:
      restartPolicy: Always
      affinity:
        podAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels:
                  app: vllm-qwen-7b
              topologyKey: kubernetes.io/hostname
      containers:
      - name: vllm
        image: vllm/vllm-openai:latest
        ports:
        - containerPort: 8001
        env:
        - name: HUGGING_FACE_HUB_TOKEN
          valueFrom:
            secretKeyRef:
              name: huggingface-secret
              key: HF_TOKEN
        command: ["/bin/bash"]
        args:
        - -c
        - |
          # Start vLLM server with Qwen2 7B Instruct model
          # Fixed port so a single Service can target both replicas
          echo "Starting vLLM on port 8001 for pod $HOSTNAME"
          vllm serve Qwen/Qwen2-7B-Instruct --host 0.0.0.0 --port 8001 --max-model-len 4096 --gpu-memory-utilization 0.8
        resources:
          limits:
            nvidia.com/mig-3g.20gb: 1
          requests:
            memory: "8Gi"
            cpu: "2"
---
apiVersion: v1
kind: Service
metadata:
  name: vllm-qwen-7b
spec:
  selector:
    app: vllm-qwen-7b
  ports:
  - port: 8001
    targetPort: 8001
    name: http
```

Apply and verify both replicas:

```bash
kubectl apply -f qwen.yaml
kubectl get pods -l app=vllm-qwen-7b -o wide
kubectl port-forward svc/vllm-qwen-7b 8001:8001 &
curl -X POST http://localhost:8001/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen/Qwen2-7B-Instruct",
    "prompt": "Answer in one word. Capital of United Kingdom is,",
    "max_tokens": 150,
    "temperature": 0.7
  }'
```

Actual output on the cluster:

```
NAME                            READY   STATUS    RESTARTS   AGE    IP            NODE                                     NOMINATED NODE   READINESS GATES
vllm-qwen-7b-847486497b-7qmgg   1/1     Running   0          118m   10.128.2.34   harpatil000043jma-p5jcx-worker-f-plch8   <none>           <none>
vllm-qwen-7b-847486497b-qj5vb   1/1     Running   0          118m   10.128.2.33   harpatil000043jma-p5jcx-worker-f-plch8   <none>           <none>
```

Sample Response:

```json
{
  "id": "cmpl-74a2a1444d5c415db1a7945879eb44ac",
  "object": "text_completion",
  "created": 1756412318,
  "model": "Qwen/Qwen2-7B-Instruct",
  "choices": [
    {
      "index": 0,
      "text": " London. \n\nStep-by-step justification:\n1. Identify the question asks for the capital of the United Kingdom.\n2. Recall that London is the capital city of the United Kingdom.\n3. Provide the answer in a single word: London.",
      "logprobs": null,
      "finish_reason": "stop",
      "stop_reason": null,
      "prompt_logprobs": null
    }
  ],
  "service_tier": null,
  "system_fingerprint": null,
  "usage": {
    "prompt_tokens": 13,
    "total_tokens": 62,
    "completion_tokens": 49,
    "prompt_tokens_details": null
  },
  "kv_transfer_params": null
}
```

You get two capable assistants sharing one card with clean isolation and predictable latency.

### Demo 3: GPT‑OSS 20B on a Full Slice

Some jobs need a lot of room. Here we dedicate nearly the whole GPU to a single model by requesting a `7g.40gb` profile. We use GPT‑OSS 20B here to illustrate a full GPU configuration; the goal is to show using the entire GPU, not model size.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: vllm-gpt-oss-20b
  labels:
    app: vllm-gpt-oss-20b
spec:
  restartPolicy: OnFailure
  containers:
  - name: vllm
    image: vllm/vllm-openai:latest
    ports:
    - containerPort: 8000
    env:
    - name: HUGGING_FACE_HUB_TOKEN
      valueFrom:
        secretKeyRef:
          name: huggingface-secret
          key: HF_TOKEN
    command: ["/bin/bash"]
    args:
    - -c
    - |
      # Install latest Transformers from source to support gpt_oss model type
      pip install git+https://github.com/huggingface/transformers.git

      # Start vLLM server with GPT-OSS model
      # GPU memory utilization reduced to 0.6 to fit in 40GB MIG slice
      vllm serve openai/gpt-oss-20b --host 0.0.0.0 --port 8000 --max-model-len 2048 --gpu-memory-utilization 0.6
    resources:
      limits:
        nvidia.com/mig-7g.40gb: 1
      requests:
        memory: "8Gi"
        cpu: "2"
---
apiVersion: v1
kind: Service
metadata:
  name: vllm-gpt-oss-20b-service
spec:
  type: ClusterIP
  ports:
  - port: 8000
    targetPort: 8000
    name: http
  selector:
    app: vllm-gpt-oss-20b
```

Deploy and test:

```bash
kubectl apply -f samples/vllm_gpt_oss_20b.yaml
kubectl get pods vllm-gpt-oss-20b -o wide
kubectl port-forward svc/vllm-gpt-oss-20b-service 8002:8000 &
curl -X POST http://localhost:8002/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "openai/gpt-oss-20b",
    "prompt": "Answer in one word. Capital of Portugal is,",
    "max_tokens": 200,
    "temperature": 0.7
  }'
```

Actual output on the cluster:

```
NAME               READY   STATUS    RESTARTS   AGE    IP            NODE                                     NOMINATED NODE   READINESS GATES
vllm-gpt-oss-20b   1/1     Running   0          155m   10.131.0.52   harpatil000043jma-p5jcx-worker-f-wj6tk   <none>           <none>
```

Sample Response:

```json
{
  "id": "cmpl-670cf60fd2ce4bb9a5b6021e1f97609d",
  "object": "text_completion",
  "created": 1756412724,
  "model": "openai/gpt-oss-20b",
  "choices": [
    {
      "index": 0,
      "text": " \"Lisbon\". But the letter L is missing from the sentence. So we could say \"Lisbon\". That is a city. \"Lisbon\" is the capital. But the clue says answer in one word. So \"Lisbon\" is fine. But is \"Lisbon\" the letter missing? No. But it's the capital of Portugal. So that fits the second part. So we choose \"Lisbon\".\n\nBut maybe they want \"Lisbon\" because it's the capital of Portugal. So answer: \"Lisbon\". That is one word. So the answer is \"Lisbon\". That fits the instruction to answer in one word.\n\nThus the answer: \"Lisbon\". But we need to check if the letter missing is L. Thus the missing letter is L. So the answer is \"Lisbon\". That would satisfy both clues.\n\nBut one might think the puzzle expects the answer \"Lisbon\". Because the missing letter is the first letter of the capital. So it's a pun",
      "logprobs": null,
      "finish_reason": "length",
      "stop_reason": null,
      "prompt_logprobs": null
    }
  ],
  "service_tier": null,
  "system_fingerprint": null,
  "usage": {
    "prompt_tokens": 10,
    "total_tokens": 210,
    "completion_tokens": 200,
    "prompt_tokens_details": null
  },
  "kv_transfer_params": null
}
```

You’ve now seen the full spectrum, from micro‑slices to a near full GPU workload, on the same cluster.

## What’s Happening Behind the Scenes

When a pod requests a MIG resource like `nvidia.com/mig-3g.20gb`, the DAS Operator coordinates with the GPU Operator and the node to create that precise slice on demand. The container starts with the slice attached; when the pod terminates, DAS cleans the slice up. The whole dance stays Kubernetes‑native: you describe resources; the platform orchestrates hardware to match.

To make it tangible, here’s a real snapshot from the cluster after deploying the three demos, using the NVIDIA driver DaemonSet to run `nvidia-smi -L` per node:

```text
=== Node: harpatil000043jma-p5jcx-worker-f-7r4b8 ===
GPU 0: NVIDIA A100-SXM4-40GB
  MIG 1g.5gb Device 0
  MIG 1g.5gb Device 1
  MIG 1g.5gb Device 2
  MIG 1g.5gb Device 3
  MIG 1g.5gb Device 4
  MIG 1g.5gb Device 5
  MIG 1g.5gb Device 6

=== Node: harpatil000043jma-p5jcx-worker-f-plch8 ===
GPU 0: NVIDIA A100-SXM4-40GB
  MIG 3g.20gb Device 0
  MIG 3g.20gb Device 1

=== Node: harpatil000043jma-p5jcx-worker-f-wj6tk ===
GPU 0: NVIDIA A100-SXM4-40GB
  MIG 7g.40gb Device 0
```

## Scale Without the Drama

Once you’re comfortable with the patterns above, the rest looks like standard Kubernetes:

- Want more throughput? Add replicas of small slices to pack the card and raise utilization without crosstalk.
- Different teams, different needs? Assign slice sizes that match their models and SLOs for clean tenancy and predictable costs.
- Prefer native tools? Keep using Deployments, HPAs, and your existing observability stack. The only new thing you request is the MIG resource.

## What’s Next: Queues and Fair‑Share with Kueue

Many platforms want queue‑based, fair‑share scheduling for ML and batch jobs. Kubernetes Kueue brings queuing, quotas, and admission control on top of Jobs and custom workloads. It pairs naturally with DAS: Kueue admits work when capacity is available, and DAS creates the right slice just in time. The outcome is higher utilization, better fairness, and simpler day‑2 operations.

## Wrap‑Up

MIG turns one GPU into many. The DAS Operator brings that power to OpenShift in an on‑demand, developer‑friendly way: request a slice, run your model, and move on, with no GPU babysitting required. Whether you’re shipping multiple small LLMs or dedicating a card to a single giant, dynamic slicing keeps the cluster busy and your users happy.

Questions or want a hand trying this in your cluster? Open an issue. We’d love to help.
