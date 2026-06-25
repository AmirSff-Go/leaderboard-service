import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const errorRate = new Rate('error_rate');
const rankingDuration = new Trend('ranking_duration', true);
const submitDuration = new Trend('submit_duration', true);

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export const options = {
  stages: [
    // Warm-up
    { duration: '20s', target: 50 },
    { duration: '40s', target: 50 },
    // Level 1
    { duration: '20s', target: 100 },
    { duration: '40s', target: 100 },
    // Level 2
    { duration: '20s', target: 200 },
    { duration: '40s', target: 200 },
    // Level 3
    { duration: '20s', target: 400 },
    { duration: '40s', target: 400 },
    // Level 4
    { duration: '20s', target: 700 },
    { duration: '40s', target: 700 },
    // Peak
    { duration: '20s', target: 1000 },
    { duration: '60s', target: 1000 },
    // Cool-down
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<2000'],
    ranking_duration: ['p(95)<1000'],
  },
};

export function setup() {
  const gameRes = http.post(
    `${BASE_URL}/admin/games`,
    JSON.stringify({
      admin_password: 'admin123',
      game_name: 'Stress Test Game',
      game_desc: 'Automated load testing',
    }),
    { headers: { 'Content-Type': 'application/json' } }
  );

  if (gameRes.status !== 201) {
    throw new Error(`Failed to create game: ${gameRes.status} ${gameRes.body}`);
  }

  const token = gameRes.json('token');

  const lbRes = http.post(
    `${BASE_URL}/leaderboards`,
    JSON.stringify({
      unique_name: 'stress-lb',
      description: 'Stress test leaderboard',
      type: 'record',
      interval_seconds: 86400,
    }),
    {
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
    }
  );

  if (lbRes.status !== 201) {
    throw new Error(`Failed to create leaderboard: ${lbRes.status} ${lbRes.body}`);
  }

  // Pre-populate 500 scores so rankings always have realistic data
  const headers = {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${token}`,
  };
  for (let i = 0; i < 500; i++) {
    http.post(
      `${BASE_URL}/leaderboards/stress-lb/scores`,
      JSON.stringify({ user_id: `seed_user_${i}`, score: Math.floor(Math.random() * 1000000) }),
      { headers }
    );
  }

  console.log(`Setup complete — game created, leaderboard seeded with 500 players`);
  return { token };
}

export default function (data) {
  const { token } = data;
  const headers = {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${token}`,
  };

  const roll = Math.random();

  if (roll < 0.20) {
    // 20% — submit a score (write path: Postgres + Redis)
    const res = http.post(
      `${BASE_URL}/leaderboards/stress-lb/scores`,
      JSON.stringify({
        user_id: `vu_${__VU}`,
        score: Math.floor(Math.random() * 1000000),
      }),
      { headers }
    );
    submitDuration.add(res.timings.duration);
    const ok = check(res, { 'submit 201': (r) => r.status === 201 });
    errorRate.add(!ok);

  } else if (roll < 0.90) {
    // 70% — get top 20 rankings (cache-first read from Redis)
    const res = http.get(
      `${BASE_URL}/leaderboards/stress-lb/rankings?page=1&page_size=20`,
      { headers }
    );
    rankingDuration.add(res.timings.duration);
    const ok = check(res, { 'rankings 200': (r) => r.status === 200 });
    errorRate.add(!ok);

  } else {
    // 10% — rankings with personal rank lookup
    const res = http.get(
      `${BASE_URL}/leaderboards/stress-lb/rankings?page=1&page_size=20&user_id=vu_${__VU}`,
      { headers }
    );
    rankingDuration.add(res.timings.duration);
    const ok = check(res, { 'rankings+rank 200': (r) => r.status === 200 });
    errorRate.add(!ok);
  }

  sleep(0.1);
}
