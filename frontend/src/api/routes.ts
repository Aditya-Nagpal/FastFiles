export const apiRoutes = {
  auth: '/auth',
  files: '/file',
  user: '/user',
  search: {
    named: 'search/named',
    smart: 'search/smart',
  },
} as const;