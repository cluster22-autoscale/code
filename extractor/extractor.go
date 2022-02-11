package extractor

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"path"
	"sort"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/jaegertracing/jaeger/model"
)

const (
	spanKeyPrefix         byte = 0x80 // All span keys should have first bit set to 1
	indexKeyRange         byte = 0x0F // Secondary indexes use last 4 bits
	serviceNameIndexKey   byte = 0x81
	operationNameIndexKey byte = 0x82
	tagIndexKey           byte = 0x83
	durationIndexKey      byte = 0x84
	jsonEncoding          byte = 0x01 // Last 4 bits of the meta byte are for encoding type
	protoEncoding         byte = 0x02 // Last 4 bits of the meta byte are for encoding type
	defaultEncoding       byte = protoEncoding

	defaultNumTraces = 100
	sizeOfTraceID    = 16
	encodingTypeBits = 0x0F
)

// primary key:
// ----------------------------------------------------------------------
//	 prefix  | traceID.high |  traceID.low | startTime | spanID
// ----------------------------------------------------------------------
//     1     |      8       |       8      | time.Time | uint64
// ----------------------------------------------------------------------

// index key:
// ----------------------------------------------------------------------
//	 prefix  | value  |  startTime  | traceID.high  |  traceID.low
// ----------------------------------------------------------------------
//     1     |   ?    |  time.Time  |     8         |      8
// ----------------------------------------------------------------------
// PS:
// ----------------------------------------------------------------------
//          prefix          |             value
// ----------------------------------------------------------------------
//   serviceNameIndexKey    |     span.Process.ServiceName
//   operationNameIndexKey  |  span.Process.ServiceName+span.OperationName
//      durationIndexKey    |  model.DurationAsMicroseconds(span.Duration)
//        tagIndexKey       |  span.Process.ServiceName+kv.Key+kv.AsString()
// ----------------------------------------------------------------------
// PPS:
//   kv means:
//      1. for _, kv := range span.Tags;
//		2. for _, kv := range span.Process.Tags;
//		3. for _, log := range span.Logs:
//			 for _, kv := range log.Fields;

type TraceReader struct {
	store *badger.DB
}

func NewTraceReader(p string) *TraceReader {
	dir := path.Join(p, "key")
	valueDir := path.Join(p, "data")
	options := badger.DefaultOptions("").WithDir(dir).WithValueDir(valueDir)
	db, err := badger.Open(options)
	if err != nil {
		log.Fatal(err)
	}
	return &TraceReader{
		store: db,
	}
}

func (tr *TraceReader) Close() {
	tr.store.Close()
}

type Query struct {
	serviceName string
	startTime   time.Time
	endTime     time.Time
	numTraces   int
}

func NewQuery(serviceName string, startTime, endTime time.Time, numTraces int) *Query {
	return &Query{
		serviceName: serviceName,
		startTime:   startTime,
		endTime:     endTime,
		numTraces:   numTraces,
	}
}

func timeAsEpochMicroseconds(t time.Time) uint64 {
	return uint64(t.UnixNano() / 1000)
}

func bytesToTraceID(key []byte) model.TraceID {
	return model.TraceID{
		High: binary.BigEndian.Uint64(key[:8]),
		Low:  binary.BigEndian.Uint64(key[8:sizeOfTraceID]),
	}
}

// QueryTimeRange this function only use primary key to scan all the tables
func (tr *TraceReader) QueryTimeRange(query *Query) ([]model.TraceID, error) {
	if query.serviceName == "" {
		return tr.queryWithoutServiceName(query)
	} else {
		return tr.queryWithServiceName(query)
	}
}

func (tr *TraceReader) queryWithoutServiceName(query *Query) ([]model.TraceID, error) {
	minTimeStamp := make([]byte, 8)
	binary.BigEndian.PutUint64(minTimeStamp, timeAsEpochMicroseconds(query.startTime))

	maxTimeStamp := make([]byte, 8)
	binary.BigEndian.PutUint64(maxTimeStamp, timeAsEpochMicroseconds(query.endTime))

	traceKeys := make([][]byte, 0)

	err := tr.store.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		startIndex := []byte{spanKeyPrefix}
		var prevTraceID []byte
		for it.Seek(startIndex); it.ValidForPrefix(startIndex); it.Next() {
			item := it.Item()
			key := item.KeyCopy(nil)

			timestamp := key[sizeOfTraceID+1 : sizeOfTraceID+1+8]
			traceID := key[1 : sizeOfTraceID+1]

			if bytes.Compare(timestamp, minTimeStamp) >= 0 && bytes.Compare(timestamp, maxTimeStamp) <= 0 {
				if !bytes.Equal(traceID, prevTraceID) {
					traceKeys = append(traceKeys, key)
					prevTraceID = traceID
				}
			}
		}
		return nil
	})

	sort.Slice(traceKeys, func(k, h int) bool {
		// This sorts by timestamp to descending order
		return bytes.Compare(traceKeys[k][sizeOfTraceID+1:sizeOfTraceID+1+8], traceKeys[h][sizeOfTraceID+1:sizeOfTraceID+1+8]) > 0
	})

	sizeCount := len(traceKeys)
	if query.numTraces > 0 && query.numTraces < sizeCount {
		sizeCount = query.numTraces
	}

	traceIDs := make([]model.TraceID, sizeCount)
	for i := 0; i < sizeCount; i++ {
		traceIDs[i] = bytesToTraceID(traceKeys[i][1 : sizeOfTraceID+1])
	}
	return traceIDs, err
}

func scanFunction(it *badger.Iterator, indexPrefix []byte, timeBytesStart []byte, timeBytesEnd []byte) bool {
	if it.Valid() {
		// We can't use the indexPrefix length, because we might have the same prefixValue for non-matching cases also
		timestampStartIndex := len(it.Item().Key()) - (sizeOfTraceID + 8) // timestamp is stored with 8 bytes
		timestamp := it.Item().Key()[timestampStartIndex : timestampStartIndex+8]
		timestampInRange := bytes.Compare(timeBytesEnd, timestamp) <= 0 && bytes.Compare(timeBytesStart, timestamp) >= 0

		// Check length as well to prevent theoretical case where timestamp might match with wrong index key
		// 24 = 8(timestamp) + 16(traceid)
		if len(it.Item().Key()) != len(indexPrefix)+24 {
			return false
		}

		return bytes.HasPrefix(it.Item().Key()[:timestampStartIndex], indexPrefix) && timestampInRange
	}
	return false
}

func (tr *TraceReader) queryWithServiceName(query *Query) ([]model.TraceID, error) {
	index := make([]byte, 0)
	index = append(index, serviceNameIndexKey)
	index = append(index, []byte(query.serviceName)...)

	minTimeStamp := make([]byte, 8)
	binary.BigEndian.PutUint64(minTimeStamp, timeAsEpochMicroseconds(query.startTime))

	maxTimeStamp := make([]byte, 8)
	binary.BigEndian.PutUint64(maxTimeStamp, timeAsEpochMicroseconds(query.endTime))

	indexResults := make([][]byte, 0)
	err := tr.store.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		// iterate from the latest to the oldest
		opts.Reverse = true

		it := txn.NewIterator(opts)
		defer it.Close()

		// 8 bytes for timestamp; 1 byte for 0xFF
		startIndex := make([]byte, len(index)+8+1)
		startIndex[len(startIndex)-1] = 0xFF
		copy(startIndex, index)
		copy(startIndex[len(index):], maxTimeStamp)

		// note: reverse = true
		for it.Seek(startIndex); scanFunction(it, index, maxTimeStamp, minTimeStamp); it.Next() {
			item := it.Item()

			// ScanFunction is a prefix scanning (since we could have for example service1 & service12)
			// Now we need to match only the exact key if we want to add it
			timestampStartIndex := len(it.Item().Key()) - (sizeOfTraceID + 8) // timestamp is stored with 8 bytes
			if bytes.Equal(index, it.Item().Key()[:timestampStartIndex]) {
				traceIDBytes := item.Key()[len(item.Key())-sizeOfTraceID:]
				traceIDCopy := make([]byte, sizeOfTraceID)
				copy(traceIDCopy, traceIDBytes)
				indexResults = append(indexResults, traceIDCopy)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(indexResults, func(k, h int) bool {
		return bytes.Compare(indexResults[k], indexResults[h]) < 0
	})

	var prevTraceID []byte
	traceIDs := make([]model.TraceID, 0)
	for i := 0; i < len(indexResults); i++ {
		traceID := indexResults[i]
		if !bytes.Equal(prevTraceID, traceID) {
			traceIDs = append(traceIDs, bytesToTraceID(traceID))
			prevTraceID = traceID
		}
	}

	return traceIDs, err
}

func createPrimaryKeySeekPrefix(traceID model.TraceID) []byte {
	key := make([]byte, 1+sizeOfTraceID)
	key[0] = spanKeyPrefix
	pos := 1
	binary.BigEndian.PutUint64(key[pos:], traceID.High)
	pos += 8
	binary.BigEndian.PutUint64(key[pos:], traceID.Low)

	return key
}

func decodeValue(val []byte, encodeType byte) (*model.Span, error) {
	sp := model.Span{}
	switch encodeType {
	case jsonEncoding:
		if err := json.Unmarshal(val, &sp); err != nil {
			return nil, err
		}
	case protoEncoding:
		if err := sp.Unmarshal(val); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown encoding type: %#02x", encodeType)
	}
	return &sp, nil
}

func (tr *TraceReader) GetTraces(traceIDs []model.TraceID) ([]*model.Trace, error) {
	traces := make([]*model.Trace, 0, len(traceIDs))
	prefixes := make([][]byte, 0, len(traceIDs))

	for _, traceID := range traceIDs {
		prefixes = append(prefixes, createPrimaryKeySeekPrefix(traceID))
	}

	err := tr.store.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		var val []byte
		for _, prefix := range prefixes {
			spans := make([]*model.Span, 0, 32)

			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				item := it.Item()
				val, err := item.ValueCopy(val)
				if err != nil {
					return nil
				}

				sp, err := decodeValue(val, item.UserMeta()&encodingTypeBits)
				if err != nil {
					return err
				}
				spans = append(spans, sp)
			}
			if len(spans) > 0 {
				trace := &model.Trace{
					Spans: spans,
				}
				traces = append(traces, trace)
			}
		}
		return nil
	})
	return traces, err
}

func (tr *TraceReader) GetPercentileLatency(percentiles []float64, traces []*model.Trace, filter func(span *model.Span) bool) []time.Duration {
	latencies := make([]time.Duration, 0)
	for _, trace := range traces {
		var latency int64
		for _, span := range trace.Spans {
			if filter(span) {
				latency += int64(span.Duration)
			}
		}
		if latency != 0 {
			latencies = append(latencies, time.Duration(latency))
		}
	}
	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	results := make([]time.Duration, len(percentiles))
	for _, percentile := range percentiles {
		index := int(float64(len(latencies)) * percentile)
		results = append(results, latencies[index])
	}

	return results
}

func (tr *TraceReader) GetPercentileLatencyByOperation(percentiles []float64, traces []*model.Trace, opNames []string) map[string][]time.Duration {
	allLatencies := make(map[string][]time.Duration, 0)
	for _, opName := range opNames {
		allLatencies[opName] = make([]time.Duration, 0)
	}

	for _, trace := range traces {
		var latency time.Duration
		var opName string
		for _, span := range trace.Spans {
			if span.SpanID.String() == span.TraceID.String() {
				opName = span.OperationName
				latency = span.Duration
				break
			}
		}
		if _, ok := allLatencies[opName]; ok {
			if latency != 0 {
				allLatencies[opName] = append(allLatencies[opName], latency)
			}
		}
	}

	for _, latencies := range allLatencies {
		sort.Slice(latencies, func(i, j int) bool {
			return latencies[i] < latencies[j]
		})
	}

	results := make(map[string][]time.Duration, len(percentiles))
	for op, latencies := range allLatencies {
		for _, percentile := range percentiles {
			index := int(float64(len(latencies)) * percentile)
			results[op] = append(results[op], latencies[index])
		}
	}

	return results
}
