import { makeStyles } from '@material-ui/core';
import { OpenChoreoIcon } from '@openchoreo/backstage-design-system';

const useStyles = makeStyles({
  svg: {
    width: 'auto',
    height: 28,
  },
  path: {
    fill: '#7df3e1',
  },
});

const LogoIcon = () => {

  return (
    <OpenChoreoIcon />
  );
};

export default LogoIcon;
