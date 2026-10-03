/** Release string baked at build time from VERSION, or a CI stamp such as 1.2.3-rc. */
export const congeeVersion: string = typeof __CONGEE_VERSION__ === 'string' ? __CONGEE_VERSION__ : '';
