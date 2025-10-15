import { createBackendModule } from '@backstage/backend-plugin-api';
import {
  authProvidersExtensionPoint,
  createOAuthProviderFactory,
  createOAuthAuthenticator,
  PassportOAuthAuthenticatorHelper,
  PassportOAuthDoneCallback,
  ProfileInfo,
  AuthResolverContext,
  OAuthAuthenticatorResult,
  OAuthSession,
} from '@backstage/plugin-auth-node';
import { Strategy as OAuth2Strategy } from 'passport-oauth2';
import { stringifyEntityRef, DEFAULT_NAMESPACE } from '@backstage/catalog-model';

/**
 * JWT token payload interface for OpenChoreo tokens
 */
interface OpenChoreoTokenPayload {
  sub: string;
  username: string;
  given_name?: string;
  family_name?: string;
  group?: string;
  aud: string;
  exp: number;
  iat: number;
  iss: string;
  jti: string;
  nbf: number;
  scope?: string;
  auth_time?: number;
}

/**
 * Custom profile transform that extracts user info from JWT tokens
 */
const customProfileTransform = async (
  result: OAuthAuthenticatorResult<any>,
  _context: AuthResolverContext
): Promise<{ profile: ProfileInfo }> => {
  console.log('customProfileTransform called with result:', JSON.stringify(result, null, 2));
  
  // Extract profile information from JWT tokens since OAuth2 doesn't provide userInfo
  // The session is available directly in result.session
  const session = result.session;
  const accessToken = session?.accessToken;
  const idToken = session?.idToken;
  
  let profile: ProfileInfo = {};
  
  // Try to extract from access token first
  if (accessToken) {
    try {
      const payload: OpenChoreoTokenPayload = JSON.parse(
        Buffer.from(accessToken.split('.')[1], 'base64').toString()
      );
      profile = {
        email: payload.username, // username contains the email
        displayName: payload.given_name && payload.family_name 
          ? `${payload.given_name} ${payload.family_name}` 
          : payload.username,
        picture: undefined, // Not available in the token
        ...profile
      };
    } catch (error) {
      console.warn('Failed to decode access token for profile:', error);
    }
  }
  
  // Fallback to ID token if access token didn't work
  if (!profile.email && idToken) {
    try {
      const payload: OpenChoreoTokenPayload = JSON.parse(
        Buffer.from(idToken.split('.')[1], 'base64').toString()
      );
      console.log('ID token payload:', JSON.stringify(payload, null, 2));
      profile = {
        email: payload.username,
        displayName: payload.given_name && payload.family_name 
          ? `${payload.given_name} ${payload.family_name}` 
          : payload.username,
        picture: undefined,
        ...profile
      };
    } catch (error) {
      console.warn('Failed to decode ID token for profile:', error);
    }
  }
  
  console.log('Final profile:', JSON.stringify(profile, null, 2));
  return { profile };
};

/**
 * Custom OAuth authenticator for OpenChoreo Default IDP
 * Uses OAuth2 strategy without OIDC discovery endpoint
 */
export const defaultIdpAuthenticator = createOAuthAuthenticator({
  defaultProfileTransform: customProfileTransform,
  scopes: {
    required: ['openid', 'profile', 'email'],
  },

  initialize({ callbackUrl, config }) {
    const clientID = config.getString('clientId');
    const clientSecret = config.getString('clientSecret');
    const authorizationURL = config.getString('authorizationUrl');
    const tokenURL = config.getString('tokenUrl');

    const strategy = new OAuth2Strategy(
      {
        clientID,
        clientSecret,
        callbackURL: callbackUrl,
        authorizationURL,
        tokenURL,
        scope: ['openid', 'profile', 'email'],
      },
      (
        accessToken: string,
        refreshToken: string,
        params: any,
        _fullProfile: any,
        done: PassportOAuthDoneCallback,
      ) => {
        // Create OAuthSession object that matches the expected structure
        const session: OAuthSession = {
          accessToken,
          tokenType: 'Bearer',
          idToken: params.id_token,
          scope: params.scope || 'openid profile email',
          expiresInSeconds: params.expires_in,
          refreshToken,
        };

        // Create a minimal PassportProfile for compatibility
        const passportProfile = {
          provider: 'default-idp',
          id: 'temp-id', // Will be replaced by profile transform
          displayName: 'temp-display-name', // Will be replaced by profile transform
        };

        // Pass the session as fullProfile and also include it in params
        done(undefined, {
          fullProfile: passportProfile,
          accessToken,
          params: {
            ...params,
            session,
          },
        }, { refreshToken });
      },
    );

    return PassportOAuthAuthenticatorHelper.from(strategy);
  },

  async start(input, helper) {
    return helper.start(input, {});
  },

  async authenticate(input, helper) {
    return helper.authenticate(input);
  },

  async refresh(input, helper) {
    return helper.refresh(input);
  },
});

/**
 * Default IDP auth provider module for OpenChoreo
 * Custom OAuth provider without OIDC discovery endpoint
 */
export const OpenChoreoDefaultAuthModule = createBackendModule({
  pluginId: 'auth',
  moduleId: 'default-idp',
  register(reg) {
    reg.registerInit({
      deps: {
        providers: authProvidersExtensionPoint,
      },
      async init({ providers }) {
        providers.registerProvider({
          providerId: 'default-idp',
          factory: createOAuthProviderFactory({
            authenticator: defaultIdpAuthenticator,
            signInResolver: async (info, ctx) => {
              const { profile } = info;

              // Handle case where profile might be undefined
              if (!profile || !profile.email) {
                throw new Error('User profile/email is undefined. Check if customProfileTransform is working correctly.');
              }

              // Extract groups from access token (where the group claim is located)
              const accessToken = (info.result as any).session?.accessToken;

              let groups: string[] = [];
              if (accessToken) {
                // Decode JWT to get claims (simple base64 decode, no verification needed here)
                try {
                  const payload = JSON.parse(
                    Buffer.from(accessToken.split('.')[1], 'base64').toString()
                  );
                  // Extract group from access token - it's a single string, not an array
                  const group = payload.group;
                  if (group) {
                    groups = [group];
                  }
                } catch (error) {
                  console.warn('Failed to decode access token for group extraction:', error);
                  // Silently continue if access token decode fails
                }
              }

              // Build entity references
              const userEntityRef = stringifyEntityRef({
                kind: 'User',
                namespace: DEFAULT_NAMESPACE,
                name: profile.email,
              });

              const ownershipEntityRefs = [
                userEntityRef,
                ...groups.map(group =>
                  stringifyEntityRef({
                    kind: 'Group',
                    namespace: DEFAULT_NAMESPACE,
                    name: group.toLowerCase(),
                  })
                ),
              ];

              // Issue token with user and group ownership
              return ctx.issueToken({
                claims: {
                  sub: userEntityRef,
                  ent: ownershipEntityRefs,
                },
              });
            },
          }),
        });
      },
    });
  },
});