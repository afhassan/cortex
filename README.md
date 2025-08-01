<p align="center"><img src="images/logo.png" alt="Cortex Logo"></p>

[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/cortexproject/cortex/badge)](https://scorecard.dev/viewer/?uri=github.com/cortexproject/cortex)
[![CI](https://github.com/cortexproject/cortex/workflows/ci/badge.svg)](https://github.com/cortexproject/cortex/actions)
[![GoDoc](https://godoc.org/github.com/cortexproject/cortex?status.svg)](https://godoc.org/github.com/cortexproject/cortex)
<a href="https://goreportcard.com/report/github.com/cortexproject/cortex"><img src="https://goreportcard.com/badge/github.com/cortexproject/cortex" alt="Go Report Card" /></a>
<a href="https://cloud-native.slack.com/messages/cortex/"><img src="https://img.shields.io/badge/join%20slack-%23cortex-brightgreen.svg" alt="Slack" /></a>
<a href="https://bestpractices.coreinfrastructure.org/projects/6681"><img src="https://bestpractices.coreinfrastructure.org/projects/6681/badge"></a>
[![CLOMonitor](https://img.shields.io/endpoint?url=https://clomonitor.io/api/projects/cncf/cortex/badge)](https://clomonitor.io/projects/cncf/cortex)


# Cortex

Cortex is a horizontally scalable, highly available, multi-tenant, long-term storage solution for [Prometheus](https://prometheus.io) and [OpenTelemetry Metrics](https://opentelemetry.io/docs/specs/otel/metrics/).

## Features

- **Horizontally scalable:** Cortex can run across multiple machines in a cluster, exceeding the throughput and storage of a single machine.
- **Highly available:** When run in a cluster, Cortex can replicate data between machines.
- **Multi-tenant:** Cortex can isolate data and queries from multiple different independent Prometheus sources in a single cluster.
- **Long-term storage:** Cortex supports S3, GCS, Swift and Microsoft Azure for long-term storage of metric data.

## Documentation

- [Getting Started](https://cortexmetrics.io/docs/getting-started/)
- [Architecture Overview](https://cortexmetrics.io/docs/architecture/)
- [Configuration](https://cortexmetrics.io/docs/configuration/)
- [Guides](https://cortexmetrics.io/docs/guides/)
- [Security](https://cortexmetrics.io/docs/guides/security/)
- [Contributing](https://cortexmetrics.io/docs/contributing/)

## Community and Support

If you have any questions about Cortex, you can:

- Ask a question on the [Cortex Slack channel](https://cloud-native.slack.com/messages/cortex/). To invite yourself to
  the CNCF Slack, visit http://slack.cncf.io/.
- [File an issue](https://github.com/cortexproject/cortex/issues/new).
- Email [cortex-users@lists.cncf.io](mailto:cortex-users@lists.cncf.io).

Your feedback is always welcome.

For security issues see https://github.com/cortexproject/cortex/security/policy

## Engage with Our Community

We invite you to participate in the Cortex Community Calls, an exciting opportunity to connect with fellow
developers and enthusiasts. These meetings are held every 4 weeks on Thursdays at 1700 UTC,
providing a platform for open discussion, collaboration, and knowledge sharing.

Our meeting notes are meticulously documented and can be
accessed [here](https://docs.google.com/document/d/1shtXSAqp3t7fiC-9uZcKkq3mgwsItAJlH6YW6x1joZo/edit), offering a
comprehensive overview of the topics discussed and decisions made.

To ensure you never miss a meeting, we've made it easy for you to keep track:

- View the Cortex Community Call schedule in your
  browser [here](https://zoom-lfx.platform.linuxfoundation.org/meetings/cortex?view=month).
- Alternatively, download the .ics
  file [here](https://webcal.prod.itx.linuxfoundation.org/lfx/a092M00001IfTjPQAV) for
  use with any calendar application or service that supports the iCal format.

Join us in shaping the future of Cortex, and let's build something amazing together!

## Further reading

### Talks

- Apr 2025 KubeCon talk "Cortex: Insights, Updates and Roadmap" ([video](https://youtu.be/3aUg2qxfoZU), [slides](https://static.sched.com/hosted_files/kccnceu2025/6c/Cortex%20Talk%20KubeCon%20EU%202025.pdf))
- Apr 2025 KubeCon talk "Taming 50 Billion Time Series: Operating Global-Scale Prometheus Deployments on Kubernetes" ([video](https://youtu.be/OqLpKJwKZlk), [slides](https://static.sched.com/hosted_files/kccnceu2025/b2/kubecon%20-%2050b%20-%20final.pdf))
- Nov 2024 KubeCon talk "Cortex Intro: Multi-Tenant Scalable Prometheus" ([video](https://youtu.be/OGAEWCoM6Tw), [slides](https://static.sched.com/hosted_files/kccncna2024/0f/Cortex%20Talk%20KubeCon%20US%202024.pdf))
- Mar 2024 KubeCon talk "Cortex Intro: Multi-Tenant Scalable Prometheus" ([video](https://youtu.be/by538PPSPQ0), [slides](https://static.sched.com/hosted_files/kccnceu2024/a1/Cortex%20Talk%20KubeConEU24.pptx.pdf))
- Apr 2023 KubeCon talk "How to Run a Rock Solid Multi-Tenant Prometheus" ([video](https://youtu.be/Pl5hEoRPLJU), [slides](https://static.sched.com/hosted_files/kccnceu2023/49/Kubecon2023.pptx.pdf))
- Oct 2022 KubeCon talk "Current State and the Future of Cortex" ([video](https://youtu.be/u1SfBAGWHgQ), [slides](https://static.sched.com/hosted_files/kccncna2022/93/KubeCon%20%2B%20CloudNativeCon%20NA%202022%20PowerPoint%20-%20Cortex.pdf))
- Oct 2021 KubeCon talk "Cortex: Intro and Production Tips" ([video](https://youtu.be/zNE_kGcUGuI), [slides](https://static.sched.com/hosted_files/kccncna2021/8e/KubeCon%202021%20NA%20Cortex%20Maintainer.pdf))
- Sep 2020 KubeCon talk "Scaling Prometheus: How We Got Some Thanos Into Cortex" ([video](https://www.youtube.com/watch?v=Z5OJzRogAS4), [slides](https://static.sched.com/hosted_files/kccnceu20/ec/2020-08%20-%20KubeCon%20EU%20-%20Cortex%20blocks%20storage.pdf))
- Jul 2020 PromCon talk "Sharing is Caring: Leveraging Open Source to Improve Cortex & Thanos" ([video](https://www.youtube.com/watch?v=2oTLouUvsac), [slides](https://docs.google.com/presentation/d/1OuKYD7-k9Grb7unppYycdmVGWN0Bo0UwdJRySOoPdpg/edit))
- Nov 2019 KubeCon talks "[Cortex 101: Horizontally Scalable Long Term Storage for Prometheus][kubecon-cortex-101]" ([video][kubecon-cortex-101-video], [slides][kubecon-cortex-101-slides]), "[Configuring Cortex for Max
  Performance][kubecon-cortex-201]" ([video][kubecon-cortex-201-video], [slides][kubecon-cortex-201-slides], [write up][kubecon-cortex-201-writeup]) and "[Blazin' Fast PromQL][kubecon-blazin]" ([slides][kubecon-blazin-slides], [video][kubecon-blazin-video], [write up][kubecon-blazin-writeup])
- Nov 2019 PromCon talk "[Two Households, Both Alike in Dignity: Cortex and Thanos][promcon-two-households]" ([video][promcon-two-households-video], [slides][promcon-two-households-slides], [write up][promcon-two-households-writeup])
- May 2019 KubeCon talks; "[Cortex: Intro][kubecon-cortex-intro]" ([video][kubecon-cortex-intro-video], [slides][kubecon-cortex-intro-slides], [blog post][kubecon-cortex-intro-blog]) and "[Cortex: Deep Dive][kubecon-cortex-deepdive]" ([video][kubecon-cortex-deepdive-video], [slides][kubecon-cortex-deepdive-slides])
- Nov 2018 CloudNative London meetup talk; "Cortex: Horizontally Scalable, Highly Available Prometheus" ([slides][cloudnative-london-2018-slides])
- Aug 2018 PromCon panel; "[Prometheus Long-Term Storage Approaches][promcon-2018-panel]" ([video][promcon-2018-video])
- Dec 2018 KubeCon talk; "[Cortex: Infinitely Scalable Prometheus][kubecon-2018-talk]" ([video][kubecon-2018-video], [slides][kubecon-2018-slides])
- Aug 2017 PromCon talk; "[Cortex: Prometheus as a Service, One Year On][promcon-2017-talk]" ([video][promcon-2017-video], [slides][promcon-2017-slides], write up [part 1][promcon-2017-writeup-1], [part 2][promcon-2017-writeup-2], [part 3][promcon-2017-writeup-3])
- Jun 2017 Prometheus London meetup talk; "Cortex: open-source, horizontally-scalable, distributed Prometheus" ([video][prometheus-london-2017-video])
- Dec 2016 KubeCon talk; "Weave Cortex: Multi-tenant, horizontally scalable Prometheus as a Service" ([video][kubecon-2016-video], [slides][kubecon-2016-slides])
- Aug 2016 PromCon talk; "Project Frankenstein: Multitenant, Scale-Out Prometheus": ([video][promcon-2016-video], [slides][promcon-2016-slides])

### Blog Posts

- Dec 2020 blog post "[How AWS and Grafana Labs are scaling Cortex for the cloud](https://aws.amazon.com/blogs/opensource/how-aws-and-grafana-labs-are-scaling-cortex-for-the-cloud/)"
- Oct 2020 blog post "[How to switch Cortex from chunks to blocks storage (and why you won't look back)](https://grafana.com/blog/2020/10/19/how-to-switch-cortex-from-chunks-to-blocks-storage-and-why-you-wont-look-back/)"
- Oct 2020 blog post "[Now GA: Cortex blocks storage for running Prometheus at scale with reduced operational complexity](https://grafana.com/blog/2020/10/06/now-ga-cortex-blocks-storage-for-running-prometheus-at-scale-with-reduced-operational-complexity/)"
- Sep 2020 blog post "[A Tale of Tail Latencies](https://www.weave.works/blog/a-tale-of-tail-latencies)"
- Aug 2020 blog post "[Scaling Prometheus: How we're pushing Cortex blocks storage to its limit and beyond](https://grafana.com/blog/2020/08/12/scaling-prometheus-how-were-pushing-cortex-blocks-storage-to-its-limit-and-beyond/)"
- Jul 2020 blog post "[How blocks storage in Cortex reduces operational complexity for running Prometheus at massive scale](https://grafana.com/blog/2020/07/29/how-blocks-storage-in-cortex-reduces-operational-complexity-for-running-prometheus-at-massive-scale/)"
- Mar 2020 blog post "[Cortex: Zone Aware Replication](https://kenhaines.net/cortex-zone-aware-replication/)"
- Mar 2020 blog post "[How we're using gossip to improve Cortex and Loki availability](https://grafana.com/blog/2020/03/25/how-were-using-gossip-to-improve-cortex-and-loki-availability/)"
- Jan 2020 blog post "[The Future of Cortex: Into the Next Decade](https://grafana.com/blog/2020/01/21/the-future-of-cortex-into-the-next-decade/)"
- Feb 2019 blog post & podcast; "[Prometheus Scalability with Bryan Boreham][prometheus-scalability]" ([podcast][prometheus-scalability-podcast])
- Feb 2019 blog post; "[How Aspen Mesh Runs Cortex in Production][aspen-mesh-2019]"
- Dec 2018 CNCF blog post; "[Cortex: a multi-tenant, horizontally scalable Prometheus-as-a-Service][cncf-2018-blog]"
- Nov 2018 CNCF TOC Presentation; "Horizontally Scalable, Multi-tenant Prometheus" ([slides][cncf-toc-presentation])
- Sept 2018 blog post; "[What is Cortex?][what-is-cortex]"
- Jul 2018 design doc; "[Cortex Query Optimisations][cortex-query-optimisation-2018]"
- Jun 2016 design document; "[Project Frankenstein: A Multi Tenant, Scale Out Prometheus](http://goo.gl/prdUYV)"

[kubecon-cortex-101]: https://kccncna19.sched.com/event/UaiH/cortex-101-horizontally-scalable-long-term-storage-for-prometheus-chris-marchbanks-splunk
[kubecon-cortex-101-video]: https://www.youtube.com/watch?v=f8GmbH0U_kI
[kubecon-cortex-101-slides]: https://static.sched.com/hosted_files/kccncna19/92/cortex_101.pdf
[kubecon-cortex-201]: https://kccncna19.sched.com/event/UagC/performance-tuning-and-day-2-operations-goutham-veeramachaneni-grafana-labs
[kubecon-cortex-201-slides]: https://static.sched.com/hosted_files/kccncna19/87/Taming%20Cortex_%20Configuring%20for%20maximum%20performance%281%29.pdf
[kubecon-cortex-201-video]: https://www.youtube.com/watch?v=VuE5aDHDexU
[kubecon-cortex-201-writeup]: https://grafana.com/blog/2019/12/02/kubecon-recap-configuring-cortex-for-maximum-performance-at-scale/
[kubecon-blazin]: https://kccncna19.sched.com/event/UaWT/blazin-fast-promql-tom-wilkie-grafana-labs
[kubecon-blazin-slides]: https://static.sched.com/hosted_files/kccncna19/0b/2019-11%20Blazin%27%20Fast%20PromQL.pdf
[kubecon-blazin-video]: https://www.youtube.com/watch?v=yYgdZyeBOck
[kubecon-blazin-writeup]: https://grafana.com/blog/2019/09/19/how-to-get-blazin-fast-promql/
[promcon-two-households]: https://promcon.io/2019-munich/talks/two-households-both-alike-in-dignity-cortex-and-thanos/
[promcon-two-households-video]: https://www.youtube.com/watch?v=KmJnmd3K3Ws&feature=youtu.be
[promcon-two-households-slides]: https://promcon.io/2019-munich/slides/two-households-both-alike-in-dignity-cortex-and-thanos.pdf
[promcon-two-households-writeup]: https://grafana.com/blog/2019/11/21/promcon-recap-two-households-both-alike-in-dignity-cortex-and-thanos/
[kubecon-cortex-intro]: https://kccnceu19.sched.com/event/MPhX/intro-cortex-tom-wilkie-grafana-labs-bryan-boreham-weaveworks
[kubecon-cortex-intro-video]: https://www.youtube.com/watch?v=_7Wnta-3-W0
[kubecon-cortex-intro-slides]: https://static.sched.com/hosted_files/kccnceu19/af/Cortex%20Intro%20KubeCon%20EU%202019.pdf
[kubecon-cortex-intro-blog]: https://grafana.com/blog/2019/05/21/grafana-labs-at-kubecon-the-latest-on-cortex/
[kubecon-cortex-deepdive]: https://kccnceu19.sched.com/event/MPjK/deep-dive-cortex-tom-wilkie-grafana-labs-bryan-boreham-weaveworks
[kubecon-cortex-deepdive-video]: https://www.youtube.com/watch?v=mYyFT4ChHio
[kubecon-cortex-deepdive-slides]: https://static.sched.com/hosted_files/kccnceu19/52/Cortex%20Deep%20Dive%20KubeCon%20EU%202019.pdf
[prometheus-scalability]: https://www.weave.works/blog/prometheus-scalability-with-bryan-boreham
[prometheus-scalability-podcast]: https://softwareengineeringdaily.com/2019/01/21/prometheus-scalability-with-bryan-boreham/
[aspen-mesh-2019]: https://www.weave.works/blog/how-aspen-mesh-runs-cortex-in-production
[kubecon-2018-talk]: https://kccna18.sched.com/event/GrXL/cortex-infinitely-scalable-prometheus-bryan-boreham-weaveworks
[kubecon-2018-video]: https://www.youtube.com/watch?v=iyN40FsRQEo
[kubecon-2018-slides]: https://static.sched.com/hosted_files/kccna18/9b/Cortex%20CloudNativeCon%202018.pdf
[cloudnative-london-2018-slides]: https://www.slideshare.net/grafana/cortex-horizontally-scalable-highly-available-prometheus
[cncf-2018-blog]: https://www.cncf.io/blog/2018/12/18/cortex-a-multi-tenant-horizontally-scalable-prometheus-as-a-service/
[cncf-toc-presentation]: https://docs.google.com/presentation/d/190oIFgujktVYxWZLhLYN4q8p9dtQYoe4sxHgn4deBSI/edit#slide=id.g3b8e2d6f7e_0_6
[what-is-cortex]: https://medium.com/weaveworks/what-is-cortex-2c30bcbd247d
[promcon-2018-panel]: https://promcon.io/2018-munich/talks/panel-discussion-prometheus-long-term-storage-approaches/
[promcon-2018-video]: https://www.youtube.com/watch?v=3pTG_N8yGSU
[prometheus-london-2017-video]: https://www.youtube.com/watch?v=Xi4jq2IUbLs
[promcon-2017-talk]: https://promcon.io/2017-munich/talks/cortex-prometheus-as-a-service-one-year-on/
[promcon-2017-video]: https://www.youtube.com/watch?v=_8DmPW4iQBQ
[promcon-2017-slides]: https://promcon.io/2017-munich/slides/cortex-prometheus-as-a-service-one-year-on.pdf
[promcon-2017-writeup-1]: https://kausal.co/blog/cortex-prometheus-aas-promcon-1/
[promcon-2017-writeup-2]: https://kausal.co/blog/cortex-prometheus-aas-promcon-2/
[promcon-2017-writeup-3]: https://kausal.co/blog/cortex-prometheus-aas-promcon-3/
[cortex-query-optimisation-2018]: https://docs.google.com/document/d/1lsvSkv0tiAMPQv-V8vI2LZ8f4i9JuTRsuPI_i-XcAqY
[kubecon-2016-video]: https://www.youtube.com/watch?v=9Uctgnazfwk
[kubecon-2016-slides]: http://www.slideshare.net/weaveworks/weave-cortex-multitenant-horizontally-scalable-prometheus-as-a-service
[promcon-2016-video]: https://youtu.be/3Tb4Wc0kfCM
[promcon-2016-slides]: http://www.slideshare.net/weaveworks/project-frankenstein-a-multitenant-horizontally-scalable-prometheus-as-a-service

## Hosted Cortex

### Amazon Managed Service for Prometheus (AMP)

[Amazon Managed Service for Prometheus (AMP)](https://aws.amazon.com/prometheus/) is a Prometheus-compatible monitoring service that makes it easy to monitor containerized applications at scale. It is a highly available, secure, and managed monitoring service for your containers. Get started [here](https://console.aws.amazon.com/prometheus/home). To learn more about AMP, reference our [documentation](https://docs.aws.amazon.com/prometheus/latest/userguide/what-is-Amazon-Managed-Service-Prometheus.html) and [Getting Started with AMP blog](https://aws.amazon.com/blogs/mt/getting-started-amazon-managed-service-for-prometheus/).

## Emeritus Maintainers

* Peter Štibraný @pstibrany
* Marco Pracucci @pracucci
* Bryan Boreham @bboreham
* Goutham Veeramachaneni @gouthamve
* Jacob Lisi @jtlisi
* Tom Wilkie @tomwilkie
* Alvin Lin @alvinlin123

## History of Cortex

The Cortex project was started by Tom Wilkie and Julius Volz (Prometheus' co-founder) in June 2016.
