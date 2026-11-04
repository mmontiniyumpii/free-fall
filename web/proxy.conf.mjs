// Proxy del servidor de desarrollo hacia el árbitro: la app pide /arbitro/... a
// su propio servidor y este reenvía al árbitro, así no depende de que el árbitro
// habilite CORS. La URL sale de ARBITRO_URL, la misma variable que usa el bot;
// sin ella apunta al mock local.
const target = process.env.ARBITRO_URL || 'http://localhost:4000';

export default {
  '/arbitro': {
    target,
    changeOrigin: true,
    secure: true,
    pathRewrite: { '^/arbitro': '' },
  },
};
