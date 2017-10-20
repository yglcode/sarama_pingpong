pingpong using kafka sarama go package:
=======================================

steps:

* docker-compose build

* docker-compose push (to registry)

* switch env to swarm manager

* docker stack deploy -c docker-compose.yml pp

* docker svc logs -f pp_ponger (check messages)
