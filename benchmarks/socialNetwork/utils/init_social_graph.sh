ip=`kubectl -n social-network get svc nginx-thrift | awk '{print $3}' | sed -n '2p'`

cd $PROJECT/benchmarks/socialNetwork/

python3 ./scripts/init_social_graph.py --ip $ip
