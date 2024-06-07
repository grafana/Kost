# Kost FAQ

## Why Kost?

We want to pivot from being reactive to proactive about cloud spend.
This starts by informing engineers as early as possible about how their changes will impact cloud spend.
Tooling exists today to estimate the cost of terraform changes, but little exists for kubernetes.
`kost` is meant to fill that gap as the space matures.

## How accurate is Kost?

The goal is to do the napkin math for you!
We're taking the requests and multiplying it against the average cost of CPU, Memory, and Storage for the _cluster_ it will exist in.
The estimate provided is likely the _worst case_ scenario.

As of this writing, we do _not_ account for the following:
- Resource-based commitments (CUDs, Reserved Instances, etc)
- Cost of spot instances
- Deployments with Horizontal Pod Autoscaling rules will not account for the min and max replicas

See our roadmap [issue](https://github.com/grafana/deployment_tools/issues/62095) to follow progress!

## Do I need to ask for approval

*_NO_*! This is _not_ meant to act as an approval mechanism.
We firmly believe our engineers will know whats best for the services they own and operate.
The primary goal is to spread awareness of costs in an automated way.

## How do I think about these numbers?

These are at best an estimate of the change in cost for the pull request.
For brand new deployments, think of this as the "absolute worst" case for increase in spend.
See [How accurate is Kost](#how-accurate-is-kost) for more information!

## Does this mean it's alright to deploy my service?

*_YES_*! See [Do I need to ask for approval?](#do-i-need-to-ask-for-approval) for more information.