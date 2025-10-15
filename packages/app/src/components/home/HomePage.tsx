import React from 'react';
import { Content, Page, Header } from '@backstage/core-components';
import {
  HomePageStarredEntities,
  HomePageToolkit,
  TemplateBackstageLogoIcon,
} from '@backstage/plugin-home';
import { HomePageSearchBar } from '@backstage/plugin-search';
import { SearchContextProvider } from '@backstage/plugin-search-react';
import { Grid, makeStyles, Typography, Card, CardContent } from '@material-ui/core';
import { useApi, identityApiRef } from '@backstage/core-plugin-api';

const useStyles = makeStyles(theme => ({
  searchBarInput: {
    maxWidth: '60vw',
    margin: 'auto',
    backgroundColor: theme.palette.background.paper,
    borderRadius: '50px',
    boxShadow: theme.shadows[1],
  },
  searchBarOutline: {
    borderStyle: 'none',
  },
  welcomeCard: {
    minHeight: '200px',
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
  },
  groupBadge: {
    display: 'inline-block',
    padding: '4px 12px',
    margin: '4px',
    borderRadius: '12px',
    backgroundColor: theme.palette.primary.main,
    color: theme.palette.primary.contrastText,
    fontSize: '0.875rem',
  },
}));

/**
 * Custom HomePage that shows different content based on user groups
 */
export const HomePage = () => {
  const classes = useStyles();
  const identityApi = useApi(identityApiRef);
  const [userGroups, setUserGroups] = React.useState<string[]>([]);
  const [userName, setUserName] = React.useState<string>('');
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    const loadUserInfo = async () => {
      try {
        const identity = await identityApi.getBackstageIdentity();
        console.log('identity', identity);
        const ownershipRefs = identity.ownershipEntityRefs || [];
        console.log('ownershipRefs', ownershipRefs);
        // Extract group names from refs like "group:default/admins"
        const groups = ownershipRefs
          .filter(ref => ref.startsWith('group:'))
          .map(ref => ref.split('/')[1]);
        console.log('groups', groups);
        setUserGroups(groups);
        setUserName(identity.userEntityRef.split('/')[1]);
      } catch (error) {
        console.error('Failed to load user info:', error);
      } finally {
        setLoading(false);
      }
    };

    loadUserInfo();
  }, [identityApi]);

  // Determine user role based on groups
  const getUserRole = () => {
    if (userGroups.includes('platformengineer')) return 'platformengineer';
    if (userGroups.includes('developer')) return 'developer';
    return 'user';
  };

  // Get welcome message based on role
  const getWelcomeMessage = () => {
    const role = getUserRole();
    switch (role) {
      case 'platformengineer':
        return 'You have full administrative access to all features and resources.';
      case 'developer':
        return 'You can create and manage your own components and services.';
      default:
        return 'Welcome to the developer portal!';
    }
  };

  // Get quick actions based on role
  const getQuickActions = () => {
    if (userGroups.includes('admins') || userGroups.includes('managers')) {
      return [
        { title: 'Manage Users', link: '/catalog?filters[kind]=user' },
        { title: 'View All Components', link: '/catalog?filters[kind]=component' },
        { title: 'Create New Component', link: '/create' },
        { title: 'System Overview', link: '/catalog?filters[kind]=system' },
      ];
    }
    if (userGroups.includes('developers')) {
      return [
        { title: 'My Components', link: `/catalog?filters[kind]=component&filters[owners]=${userName}` },
        { title: 'Create Component', link: '/create' },
        { title: 'Browse Catalog', link: '/catalog' },
        { title: 'Documentation', link: '/docs' },
      ];
    }
    // Viewers
    return [
      { title: 'Browse Catalog', link: '/catalog' },
      { title: 'Documentation', link: '/docs' },
      { title: 'API Docs', link: '/api-docs' },
      { title: 'Tech Radar', link: '/tech-radar' },
    ];
  };

  if (loading) {
    return (
      <Page themeId="home">
        <Header title="Loading..." />
        <Content>
          <Typography>Loading user information...</Typography>
        </Content>
      </Page>
    );
  }

  return (
    <SearchContextProvider>
      <Page themeId="home">
        <Header title={`Welcome, ${userName}!`} subtitle={getUserRole()} />
        <Content>
          <Grid container justifyContent="center" spacing={6}>
            {/* Search Bar */}
            <Grid container item xs={12} justifyContent="center">
              <HomePageSearchBar
                InputProps={{
                  classes: {
                    root: classes.searchBarInput,
                    notchedOutline: classes.searchBarOutline,
                  },
                }}
                placeholder="Search"
              />
            </Grid>

            {/* Welcome Card with Role Information */}
            <Grid container item xs={12}>
              <Grid item xs={12} md={6}>
                <Card className={classes.welcomeCard}>
                  <CardContent>
                    <Typography variant="h5" gutterBottom>
                      {getWelcomeMessage()}
                    </Typography>
                    <Typography variant="body2" color="textSecondary" gutterBottom>
                      Your groups:
                    </Typography>
                    <div>
                      {userGroups.map(group => (
                        <span key={group} className={classes.groupBadge}>
                          {group}
                        </span>
                      ))}
                    </div>
                  </CardContent>
                </Card>
              </Grid>

              {/* Quick Actions based on role */}
              <Grid item xs={12} md={6}>
                <Card>
                  <CardContent>
                    <Typography variant="h6" gutterBottom>
                      Quick Actions
                    </Typography>
                    <Grid container spacing={2}>
                      {getQuickActions().map(action => (
                        <Grid item xs={6} key={action.title}>
                          <Card variant="outlined">
                            <CardContent>
                              <a
                                href={action.link}
                                style={{ textDecoration: 'none', color: 'inherit' }}
                              >
                                <Typography variant="body2">
                                  {action.title}
                                </Typography>
                              </a>
                            </CardContent>
                          </Card>
                        </Grid>
                      ))}
                    </Grid>
                  </CardContent>
                </Card>
              </Grid>
            </Grid>

            {/* Starred Entities - shown to all users */}
            <Grid container item xs={12} md={6}>
              <HomePageStarredEntities />
            </Grid>

            {/* Toolkit - conditional based on role */}
            {(userGroups.includes('admins') ||
              userGroups.includes('managers') ||
              userGroups.includes('developers')) && (
              <Grid container item xs={12} md={6}>
                <HomePageToolkit
                  tools={[
                    {
                      url: '/create',
                      label: 'Create Component',
                      icon: <TemplateBackstageLogoIcon />,
                    },
                    {
                      url: '/catalog-import',
                      label: 'Import Entity',
                      icon: <TemplateBackstageLogoIcon />,
                    },
                  ]}
                />
              </Grid>
            )}
          </Grid>
        </Content>
      </Page>
    </SearchContextProvider>
  );
};
